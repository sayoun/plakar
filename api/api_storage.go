package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gorilla/mux"
)

func storageConfiguration(w http.ResponseWriter, r *http.Request) error {
	configuration := lstore.Configuration()
	return json.NewEncoder(w).Encode(configuration)
}

func storageStates(w http.ResponseWriter, r *http.Request) error {
	vars := mux.Vars(r)
	_ = vars

	states, err := lstore.GetStates()
	if err != nil {
		return err
	}

	items := Items{
		Total: len(states),
		Items: make([]interface{}, len(states)),
	}
	for i, state := range states {
		items.Items[i] = state
	}

	return json.NewEncoder(w).Encode(items)
}

func storageState(w http.ResponseWriter, r *http.Request) error {
	stateBytes32, err := PathParamToID(r, "state")
	if err != nil {
		return err
	}

	rd, _, err := lstore.GetState(stateBytes32)
	if err != nil {
		return err
	}

	io.Copy(w, rd)
	return nil
}

func storagePackfiles(w http.ResponseWriter, r *http.Request) error {
	packfiles, err := lstore.GetPackfiles()
	if err != nil {
		return err
	}

	items := Items{
		Total: len(packfiles),
		Items: make([]interface{}, len(packfiles)),
	}
	for i, packfile := range packfiles {
		items.Items[i] = packfile
	}

	return json.NewEncoder(w).Encode(items)
}

func storagePackfile(w http.ResponseWriter, r *http.Request) error {
	packfileBytes32, err := PathParamToID(r, "packfile")
	if err != nil {
		return err
	}

	offset, offsetExists, err := QueryParamToUint32(r, "offset")
	if err != nil {
		return err
	}
	length, lengthExists, err := QueryParamToUint32(r, "length")
	if err != nil {
		return err
	}

	if (offsetExists && !lengthExists) || (!offsetExists && lengthExists) {
		param := "offset"
		if !offsetExists {
			param = "length"
		}
		return parameterError(param, MissingArgument, ErrMissingField)
	}

	var rd io.Reader
	if offsetExists && lengthExists {
		rd, _, err = lstore.GetPackfileBlob(packfileBytes32, uint32(offset), uint32(length))
		if err != nil {
			return err
		}
	} else {
		rd, _, err = lstore.GetPackfile(packfileBytes32)
		if err != nil {
			return err
		}
	}
	io.Copy(w, rd)
	return nil
}
