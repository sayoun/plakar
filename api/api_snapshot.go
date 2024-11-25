package api

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/PlakarKorp/plakar/logger"
	"github.com/PlakarKorp/plakar/objects"
	"github.com/PlakarKorp/plakar/packfile"
	"github.com/PlakarKorp/plakar/snapshot"
	"github.com/PlakarKorp/plakar/snapshot/vfs"
	"github.com/alecthomas/chroma/formatters"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/styles"
	"github.com/gorilla/mux"
)

func snapshotHeader(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	snapshotIDstr := vars["snapshot"]

	snapshotID, err := hex.DecodeString(snapshotIDstr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(snapshotID) != 32 {
		http.Error(w, "Invalid snapshot ID", http.StatusBadRequest)
		return
	}
	snapshotID32 := [32]byte{}
	copy(snapshotID32[:], snapshotID)

	snap, err := snapshot.Load(lrepository, snapshotID32)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(Item{Item: snap.Header})
}

func snapshotReader(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	snapshotIDstr := vars["snapshot"]
	path := vars["path"]

	do_highlight := false
	do_download := false

	download := r.URL.Query().Get("download")
	if download == "true" {
		do_download = true
	}

	render := r.URL.Query().Get("render")
	if render == "highlight" {
		do_highlight = true
	}

	snapshotID, err := hex.DecodeString(snapshotIDstr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(snapshotID) != 32 {
		http.Error(w, "Invalid snapshot ID", http.StatusBadRequest)
		return
	}
	snapshotID32 := [32]byte{}
	copy(snapshotID32[:], snapshotID)

	snap, err := snapshot.Load(lrepository, snapshotID32)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rd, err := snap.NewReader(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if do_download {
		w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(filepath.Base(path)))
	}

	if do_highlight {
		lexer := lexers.Match(path)
		if lexer == nil {
			lexer = lexers.Get(rd.GetContentType())
		}
		if lexer == nil {
			lexer = lexers.Fallback // Fallback if no lexer is found
		}
		formatter := formatters.Get("html")
		style := styles.Get("dracula")

		w.Header().Set("Content-Type", "text/html")

		reader := bufio.NewReader(rd)
		buffer := make([]byte, 4096) // Fixed-size buffer for chunked reading
		for {
			n, err := reader.Read(buffer) // Read up to the size of the buffer
			if n > 0 {
				chunk := string(buffer[:n])

				// Tokenize the chunk and apply syntax highlighting
				iterator, errTokenize := lexer.Tokenise(nil, chunk)
				if errTokenize != nil {
					break
				}

				errFormat := formatter.Format(w, style, iterator)
				if errFormat != nil {
					break
				}
			}

			// Check for end of file (EOF)
			if err == io.EOF {
				break
			} else if err != nil {
				break
			}
		}
	} else {

		if rd.GetContentType() != "" {
			w.Header().Set("Content-Type", rd.GetContentType())
		}

		_, err = io.Copy(w, rd)
		if err != nil {
			// Connection closed by client
			if errors.Is(err, syscall.EPIPE) {
				return
			}
			logger.Error("Failed to copy data: %s", err)
			return
		}
	}
}

func snapshotVFSBrowse(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	snapshotIDstr := vars["snapshot"]
	path := vars["path"]

	snapshotID, err := hex.DecodeString(snapshotIDstr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(snapshotID) != 32 {
		http.Error(w, "Invalid snapshot ID", http.StatusBadRequest)
		return
	}
	snapshotID32 := [32]byte{}
	copy(snapshotID32[:], snapshotID)

	snap, err := snapshot.Load(lrepository, snapshotID32)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fs, err := snap.Filesystem()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if path == "" {
		path = "/"
	}
	fsinfo, err := fs.Stat(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if dirEntry, ok := fsinfo.(*vfs.DirEntry); ok {
		json.NewEncoder(w).Encode(Item{Item: dirEntry})
		return
	} else if fileEntry, ok := fsinfo.(*vfs.FileEntry); ok {
		json.NewEncoder(w).Encode(Item{Item: fileEntry})
		return
	} else {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
}

func snapshotVFSChildren(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	snapshotIDstr := vars["snapshot"]
	path := vars["path"]

	var err error
	var sortKeys []string
	var offset int64
	var limit int64

	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")

	sortKeysStr := r.URL.Query().Get("sort")
	if sortKeysStr == "" {
		sortKeysStr = "Name"
	}

	sortKeys, err = objects.ParseFileInfoSortKeys(sortKeysStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if offsetStr != "" {
		offset, err = strconv.ParseInt(offsetStr, 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		} else if offset < 0 {
			http.Error(w, "Invalid offset", http.StatusBadRequest)
			return
		}
	}
	if limitStr != "" {
		limit, err = strconv.ParseInt(limitStr, 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		} else if limit < 0 {
			http.Error(w, "Invalid limit", http.StatusBadRequest)
			return
		}
	}

	snapshotID, err := hex.DecodeString(snapshotIDstr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(snapshotID) != 32 {
		http.Error(w, "Invalid snapshot ID", http.StatusBadRequest)
		return
	}
	snapshotID32 := [32]byte{}
	copy(snapshotID32[:], snapshotID)

	snap, err := snapshot.Load(lrepository, snapshotID32)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fs, err := snap.Filesystem()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if path == "" {
		path = "/"
	}
	fsinfo, err := fs.Stat(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if dirEntry, ok := fsinfo.(*vfs.DirEntry); !ok {
		http.Error(w, "not a directory", http.StatusBadRequest)
		return
	} else {
		fileInfos := make([]objects.FileInfo, 0, len(dirEntry.Children))
		children := make(map[string]vfs.ChildEntry)
		for _, child := range dirEntry.Children {
			fileInfos = append(fileInfos, child.Stat())
			children[child.Stat().Name()] = child
		}

		if limit == 0 {
			limit = int64(len(dirEntry.Children))
		}
		if err := objects.SortFileInfos(fileInfos, sortKeys); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if offset > int64(len(dirEntry.Children)) {
			fileInfos = []objects.FileInfo{}
		} else if offset+limit > int64(len(dirEntry.Children)) {
			fileInfos = fileInfos[offset:]
		} else {
			fileInfos = fileInfos[offset : offset+limit]
		}

		childEntries := make([]vfs.ChildEntry, 0, len(fileInfos))
		for _, fileInfo := range fileInfos {
			childEntries = append(childEntries, children[fileInfo.Name()])
		}

		items := Items{
			Total: len(dirEntry.Children),
			Items: make([]interface{}, len(childEntries)),
		}
		for i, child := range childEntries {
			items.Items[i] = child
		}
		json.NewEncoder(w).Encode(items)
	}
}

func snapshotVFSErrors(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	snapshotIDstr := vars["snapshot"]
	path := vars["path"]

	var err error
	var offset int64
	var limit int64

	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")

	sortKeysStr := r.URL.Query().Get("sort")
	if sortKeysStr == "" {
		sortKeysStr = "Name"
	}
	if sortKeysStr != "Name" && sortKeysStr != "-Name" {
		http.Error(w, "Invalid sort key", http.StatusBadRequest)
		return
	}

	if offsetStr != "" {
		offset, err = strconv.ParseInt(offsetStr, 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		} else if offset < 0 {
			http.Error(w, "Invalid offset", http.StatusBadRequest)
			return
		}
	}
	if limitStr != "" {
		limit, err = strconv.ParseInt(limitStr, 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		} else if limit < 0 {
			http.Error(w, "Invalid limit", http.StatusBadRequest)
			return
		}
	}

	snapshotID, err := hex.DecodeString(snapshotIDstr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(snapshotID) != 32 {
		http.Error(w, "Invalid snapshot ID", http.StatusBadRequest)
		return
	}
	snapshotID32 := [32]byte{}
	copy(snapshotID32[:], snapshotID)

	snap, err := snapshot.Load(lrepository, snapshotID32)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fs, err := snap.Filesystem()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if path == "" {
		path = "/"
	}
	fsinfo, err := fs.Stat(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if dirEntry, ok := fsinfo.(*vfs.DirEntry); !ok {
		http.Error(w, "not a directory", http.StatusBadRequest)
		return
	} else {
		items := Items{
			Total: 0,
			Items: make([]interface{}, 0),
		}
		if dirEntry.ErrorFirst != nil {

			if sortKeysStr == "Name" {
				iter := dirEntry.ErrorFirst
				for i := int64(0); i < limit+offset && iter != nil; i++ {
					errorEntryBytes, err := snap.GetBlob(packfile.TYPE_ERROR, *iter)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}

					errorEntry, err := vfs.ErrorEntryFromBytes(errorEntryBytes)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					iter = errorEntry.Successor

					if i < offset {
						continue
					}

					items.Total += 1
					errorEntry.Predecessor = nil
					errorEntry.Successor = nil
					items.Items = append(items.Items, errorEntry)
				}
			} else if sortKeysStr == "-Name" {
				iter := dirEntry.ErrorLast
				for i := int64(0); i < limit+offset && iter != nil; i++ {
					errorEntryBytes, err := snap.GetBlob(packfile.TYPE_ERROR, *iter)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}

					errorEntry, err := vfs.ErrorEntryFromBytes(errorEntryBytes)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					iter = errorEntry.Predecessor
					if i < offset {
						continue
					}
					items.Total += 1
					errorEntry.Predecessor = nil
					errorEntry.Successor = nil
					items.Items = append(items.Items, errorEntry)
				}
			}
		}
		json.NewEncoder(w).Encode(items)
	}
}
