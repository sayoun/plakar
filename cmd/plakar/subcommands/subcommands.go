package subcommands

import (
	"fmt"
	"sort"

	"github.com/PlakarKorp/plakar/appcontext"
	"github.com/PlakarKorp/plakar/repository"
	"github.com/vmihailenco/msgpack/v5"
)

type Subcommand interface {
	Execute(ctx *appcontext.AppContext, repo *repository.Repository) (int, error)
}

type parseArgsFn func(*appcontext.AppContext, []string) (Subcommand, error)

var subcommands map[string]parseArgsFn = make(map[string]parseArgsFn)

func Register(command string, fn parseArgsFn) {
	subcommands[command] = fn
}

func Parse(ctx *appcontext.AppContext, command string, args []string) (Subcommand, error) {
	parsefn, exists := subcommands[command]
	if !exists {
		return nil, fmt.Errorf("unknown command: %s", command)
	}
	return parsefn(ctx, args)
}

func List() []string {
	var list []string
	for command := range subcommands {
		list = append(list, command)
	}
	sort.Strings(list)
	return list
}

// RPC extends subcommands.Subcommand, but it also includes the Name() method used to identify the RPC on decoding.
type RPC interface {
	Subcommand
	Name() string
}

type encodedRPC struct {
	Name        string
	Subcommand  RPC
	StoreConfig map[string]string
}

// Encode marshals the RPC into the msgpack encoder. It prefixes the RPC with
// the Name() of the RPC. This is used to identify the RPC on decoding.
func EncodeRPC(encoder *msgpack.Encoder, cmd RPC, storeConfig map[string]string) error {
	return encoder.Encode(encodedRPC{
		Name:        cmd.Name(),
		Subcommand:  cmd,
		StoreConfig: storeConfig,
	})
}

// Decode extracts the request encoded by Encode(). It returns the name of the
// RPC and the raw bytes of the request. The raw bytes can be used by the caller
// to unmarshal the bytes with the correct struct.
func DecodeRPC(decoder *msgpack.Decoder) (string, map[string]string, []byte, error) {
	var request map[string]interface{}
	if err := decoder.Decode(&request); err != nil {
		return "", nil, nil, fmt.Errorf("failed to decode client request: %w", err)
	}

	rawRequest, err := msgpack.Marshal(request)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to marshal client request: %s", err)
	}

	name, ok := request["Name"].(string)
	if !ok {
		return "", nil, nil, fmt.Errorf("request does not contain a Name string field")
	}

	storeConfig, ok := request["StoreConfig"].(map[string]interface{})
	if !ok {
		return "", nil, nil, fmt.Errorf("request does not contain a StoreConfig field")
	}

	okStoreConfig := make(map[string]string)
	for k, v := range storeConfig {
		if str, ok := v.(string); ok {
			okStoreConfig[k] = str
		} else {
			return "", nil, nil, fmt.Errorf("StoreConfig field %s is not a string", k)
		}
	}

	return name, okStoreConfig, rawRequest, nil
}
