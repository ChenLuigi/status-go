package status

import (
	"encoding/json"
	"fmt"
	"os"
	"unsafe"

	"github.com/NaySoftware/go-fcm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/status-im/status-go/api"
	"github.com/status-im/status-go/logutils"
	"github.com/status-im/status-go/params"
	"github.com/status-im/status-go/profiling"
	"github.com/status-im/status-go/services/personal"
	"github.com/status-im/status-go/signal"
	"github.com/status-im/status-go/transactions"
	validator "gopkg.in/go-playground/validator.v9"
)

var statusBackend = api.NewStatusBackend()

// All general log messages in this package should be routed through this logger.
var logger = log.New("package", "status-go/lib")

//GenerateConfig for status node
func GenerateConfig(datadir string, networkID int) string {
	config, err := params.NewNodeConfig(datadir, "", params.FleetBeta, uint64(networkID))
	if err != nil {
		return makeJSONResponse(err)
	}

	outBytes, err := json.Marshal(config)
	if err != nil {
		return makeJSONResponse(err)
	}

	return string(outBytes)
}

//StartNode - start Status node
func StartNode(configJSON string) string {
	config, err := params.LoadNodeConfig(configJSON)
	if err != nil {
		return makeJSONResponse(err)
	}

	if err := logutils.OverrideRootLog(true, "INFO", "", true); err != nil {
		return makeJSONResponse(err)
	}

	api.RunAsync(func() error { return statusBackend.StartNode(config) })

	return makeJSONResponse(nil)
}

//StopNode - stop status node
func StopNode() string {
	api.RunAsync(statusBackend.StopNode)
	return makeJSONResponse(nil)
}

//ValidateNodeConfig validates config for status node
func ValidateNodeConfig(configJSON string) string {
	var resp APIDetailedResponse

	_, err := params.LoadNodeConfig(configJSON)

	// Convert errors to APIDetailedResponse
	switch err := err.(type) {
	case validator.ValidationErrors:
		resp = APIDetailedResponse{
			Message:     "validation: validation failed",
			FieldErrors: make([]APIFieldError, len(err)),
		}

		for i, ve := range err {
			resp.FieldErrors[i] = APIFieldError{
				Parameter: ve.Namespace(),
				Errors: []APIError{
					{
						Message: fmt.Sprintf("field validation failed on the '%s' tag", ve.Tag()),
					},
				},
			}
		}
	case error:
		resp = APIDetailedResponse{
			Message: fmt.Sprintf("validation: %s", err.Error()),
		}
	case nil:
		resp = APIDetailedResponse{
			Status: true,
		}
	}

	respJSON, err := json.Marshal(resp)
	if err != nil {
		return makeJSONResponse(err)
	}

	return string(respJSON)
}

//ResetChainData remove chain data from data directory
func ResetChainData() string {
	api.RunAsync(statusBackend.ResetChainData)
	return makeJSONResponse(nil)
}

//CallRPC calls public APIs via RPC
func CallRPC(inputJSON string) string {
	return statusBackend.CallRPC(inputJSON)
}

//CallPrivateRPC calls both public and private APIs via RPC
func CallPrivateRPC(inputJSON string) string {
	return statusBackend.CallPrivateRPC(inputJSON)
}

//CreateAccount is equivalent to creating an account from the command line,
// just modified to handle the function arg passing
func CreateAccount(password string) string {
	address, pubKey, mnemonic, err := statusBackend.AccountManager().CreateAccount(password)

	errString := ""
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		errString = err.Error()
	}

	out := AccountInfo{
		Address:  address,
		PubKey:   pubKey,
		Mnemonic: mnemonic,
		Error:    errString,
	}
	outBytes, _ := json.Marshal(out)
	return string(outBytes)
}

//CreateChildAccount creates sub-account
func CreateChildAccount(parentAddress, password string) string {
	address, pubKey, err := statusBackend.AccountManager().CreateChildAccount(parentAddress, password)

	errString := ""
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		errString = err.Error()
	}

	out := AccountInfo{
		Address: address,
		PubKey:  pubKey,
		Error:   errString,
	}
	outBytes, _ := json.Marshal(out)
	return string(outBytes)
}

//RecoverAccount re-creates master key using given details
func RecoverAccount(password, mnemonic string) string {
	address, pubKey, err := statusBackend.AccountManager().RecoverAccount(password, mnemonic)

	errString := ""
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		errString = err.Error()
	}

	out := AccountInfo{
		Address:  address,
		PubKey:   pubKey,
		Mnemonic: (mnemonic),
		Error:    errString,
	}
	outBytes, _ := json.Marshal(out)
	return string(outBytes)
}

//VerifyAccountPassword verifies account password
func VerifyAccountPassword(keyStoreDir, address, password string) string {
	_, err := statusBackend.AccountManager().VerifyAccountPassword(keyStoreDir, address, password)
	return makeJSONResponse(err)
}

//Login loads a key file (for a given address), tries to decrypt it using the password, to verify ownership
// if verified, purges all the previous identities from Whisper, and injects verified key as shh identity
func Login(address, password string) string {
	err := statusBackend.SelectAccount(address, password)
	return makeJSONResponse(err)
}

//Logout is equivalent to clearing whisper identities
func Logout() string {
	err := statusBackend.Logout()
	return makeJSONResponse(err)
}

// SignMessage unmarshals rpc params {data, address, password} and passes
// them onto backend.SignMessage
func SignMessage(rpcParams string) string {
	var params personal.SignParams
	err := json.Unmarshal([]byte(rpcParams), &params)
	if err != nil {
		return prepareJSONResponseWithCode(nil, err, codeFailedParseParams)
	}
	result, err := statusBackend.SignMessage(params)
	return prepareJSONResponse(result.String(), err)
}

// Recover unmarshals rpc params {signDataString, signedData} and passes
// them onto backend.
func Recover(rpcParams string) string {
	var params personal.RecoverParams
	err := json.Unmarshal([]byte(rpcParams), &params)
	if err != nil {
		return prepareJSONResponseWithCode(nil, err, codeFailedParseParams)
	}
	addr, err := statusBackend.Recover(params)
	return prepareJSONResponse(addr.String(), err)
}

// SendTransaction converts RPC args and calls backend.SendTransaction
func SendTransaction(txArgsJSON, password string) string {
	var params transactions.SendTxArgs
	err := json.Unmarshal([]byte(txArgsJSON), &params)
	if err != nil {
		return prepareJSONResponseWithCode(nil, err, codeFailedParseParams)
	}
	hash, err := statusBackend.SendTransaction(params, password)
	code := codeUnknown
	if c, ok := errToCodeMap[err]; ok {
		code = c
	}
	return prepareJSONResponseWithCode(hash.String(), err, code)
}

//StartCPUProfile runs pprof for cpu
func StartCPUProfile(dataDir string) string {
	err := profiling.StartCPUProfile(dataDir)
	return makeJSONResponse(err)
}

//StopCPUProfiling stops pprof for cpu
func StopCPUProfiling() string { //nolint: deadcode
	err := profiling.StopCPUProfile()
	return makeJSONResponse(err)
}

//WriteHeapProfile starts pprof for heap
func WriteHeapProfile(dataDir string) string { //nolint: deadcode
	err := profiling.WriteHeapFile(dataDir)
	return makeJSONResponse(err)
}

func makeJSONResponse(err error) string {
	errString := ""
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		errString = err.Error()
	}

	out := APIResponse{
		Error: errString,
	}
	outBytes, _ := json.Marshal(out)

	return string(outBytes)
}

// NotifyUsers sends push notifications by given tokens.
func NotifyUsers(message, payloadJSON, tokensArray string) (outCBytes string) {
	var (
		err      error
		outBytes []byte
	)
	errString := ""

	defer func() {
		out := NotifyResult{
			Status: err == nil,
			Error:  errString,
		}

		outBytes, err = json.Marshal(out)
		if err != nil {
			logger.Error("failed to marshal Notify output", "error", err)
			outCBytes = makeJSONResponse(err)
			return
		}

		outCBytes = string(outBytes)
	}()

	tokens, err := ParseJSONArray(tokensArray)
	if err != nil {
		errString = err.Error()
		return
	}

	var payload fcm.NotificationPayload
	err = json.Unmarshal([]byte(payloadJSON), &payload)
	if err != nil {
		errString = err.Error()
		return
	}

	err = statusBackend.NotifyUsers(message, payload, tokens...)
	if err != nil {
		errString = err.Error()
		return
	}

	return
}

// AddPeer adds an enode as a peer.
func AddPeer(enode string) string {
	err := statusBackend.StatusNode().AddPeer(enode)
	return makeJSONResponse(err)
}

// ConnectionChange handles network state changes as reported
// by ReactNative (see https://facebook.github.io/react-native/docs/netinfo.html)
func ConnectionChange(typ string, expensive int) {
	statusBackend.ConnectionChange(typ, expensive == 1)
}

// AppStateChange handles app state changes (background/foreground).
func AppStateChange(state string) {
	statusBackend.AppStateChange(state)
}

// SetSignalEventCallback setup geth callback to notify about new signal
func SetSignalEventCallback(cb unsafe.Pointer) {
	signal.SetSignalEventCallback(cb)
}
