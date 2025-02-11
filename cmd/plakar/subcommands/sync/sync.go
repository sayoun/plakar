/*
 * Copyright (c) 2021 Gilles Chehade <gilles@poolp.org>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package sync

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/PlakarKorp/plakar/appcontext"
	"github.com/PlakarKorp/plakar/cmd/plakar/subcommands"
	"github.com/PlakarKorp/plakar/cmd/plakar/utils"
	"github.com/PlakarKorp/plakar/encryption"
	"github.com/PlakarKorp/plakar/objects"
	"github.com/PlakarKorp/plakar/repository"
	"github.com/PlakarKorp/plakar/resources"
	"github.com/PlakarKorp/plakar/snapshot"
	"github.com/PlakarKorp/plakar/storage"
	"github.com/vmihailenco/msgpack/v5"
)

func init() {
	subcommands.Register("sync", parse_cmd_sync)
}

func parse_cmd_sync(ctx *appcontext.AppContext, repo *repository.Repository, args []string) (subcommands.Subcommand, error) {
	flags := flag.NewFlagSet("sync", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: %s [SNAPSHOT] to REPOSITORY\n", flags.Name())
		fmt.Fprintf(flags.Output(), "       %s [SNAPSHOT] from REPOSITORY\n", flags.Name())
		flags.PrintDefaults()
	}
	flags.Parse(args)
	return &Sync{
		RepositoryLocation: repo.Location(),
		RepositorySecret:   ctx.GetSecret(),
		Args:               flags.Args(),
	}, nil
}

type Sync struct {
	RepositoryLocation string
	RepositorySecret   []byte

	Args []string
}

func (cmd *Sync) Name() string {
	return "sync"
}

func (cmd *Sync) Execute(ctx *appcontext.AppContext, repo *repository.Repository) (int, error) {
	syncSnapshotID := ""
	direction := ""
	peerRepositoryPath := ""
	switch len(cmd.Args) {
	case 2:
		direction = cmd.Args[0]
		peerRepositoryPath = cmd.Args[1]

	case 3:
		syncSnapshotID = cmd.Args[0]
		direction = cmd.Args[1]
		peerRepositoryPath = cmd.Args[2]

	default:
		return 1, fmt.Errorf("usage: sync [SNAPSHOT] to|from REPOSITORY")
	}

	peerStore, peerStoreSerializedConfig, err := storage.Open(peerRepositoryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: could not open repository: %s\n", peerRepositoryPath, err)
		return 1, err
	}

	peerStoreConfig, err := storage.NewConfigurationFromWrappedBytes(peerStoreSerializedConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: could not parse configuration: %s\n", peerStore.Location(), err)
		return 1, err
	}

	var peerSecret []byte
	if peerStoreConfig.Encryption != nil {
		for {
			passphrase, err := utils.GetPassphrase("destination repository")
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}

			key, err := encryption.DeriveKey(peerStoreConfig.Encryption.KDFParams, passphrase)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}
			if !encryption.VerifyCanary(peerStoreConfig.Encryption, key) {
				fmt.Fprintf(os.Stderr, "invalid passphrase\n")
				continue
			}
			peerSecret = key
			break
		}
	}

	peerCtx := appcontext.NewAppContextFrom(ctx)
	peerCtx.SetSecret(peerSecret)
	peerRepository, err := repository.New(peerCtx, peerStore, peerStoreSerializedConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: could not open repository: %s\n", peerStore.Location(), err)
		return 1, err
	}

	var srcRepository *repository.Repository
	var dstRepository *repository.Repository

	if direction == "to" {
		srcRepository = repo
		dstRepository = peerRepository
	} else if direction == "from" {
		srcRepository = peerRepository
		dstRepository = repo
	} else if direction == "with" {
		srcRepository = repo
		dstRepository = peerRepository
	} else {
		fmt.Fprintf(os.Stderr, "%s: invalid direction, must be to, from or with\n", peerStore.Location())
		return 1, err
	}

	srcSnapshots, err := srcRepository.GetSnapshots()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: could not get snapshots from repository: %s\n", srcRepository.Location(), err)
		return 1, err
	}

	dstSnapshots, err := dstRepository.GetSnapshots()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: could not get snapshots list from repository: %s\n", dstRepository.Location(), err)
		return 1, err
	}

	_ = syncSnapshotID

	srcSnapshotsMap := make(map[objects.MAC]struct{})
	dstSnapshotsMap := make(map[objects.MAC]struct{})

	for _, snapshotID := range srcSnapshots {
		srcSnapshotsMap[snapshotID] = struct{}{}
	}

	for _, snapshotID := range dstSnapshots {
		dstSnapshotsMap[snapshotID] = struct{}{}
	}

	srcSyncList := make([]objects.MAC, 0)
	for snapshotID := range srcSnapshotsMap {
		if syncSnapshotID != "" {
			hexSnapshotID := hex.EncodeToString(snapshotID[:])
			if !strings.HasPrefix(hexSnapshotID, syncSnapshotID) {
				continue
			}
		}
		if _, exists := dstSnapshotsMap[snapshotID]; !exists {
			srcSyncList = append(srcSyncList, snapshotID)
		}
	}

	fmt.Printf("Synchronizing %d snapshots\n", len(srcSyncList))

	for _, snapshotID := range srcSyncList {
		err := synchronize(srcRepository, dstRepository, snapshotID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: could not synchronize snapshot %x from repository: %s\n", srcRepository.Location(), snapshotID, err)
		}
	}

	if direction == "with" {
		dstSyncList := make([]objects.MAC, 0)
		for snapshotID := range dstSnapshotsMap {
			if syncSnapshotID != "" {
				hexSnapshotID := hex.EncodeToString(snapshotID[:])
				if !strings.HasPrefix(hexSnapshotID, syncSnapshotID) {
					continue
				}
			}
			if _, exists := srcSnapshotsMap[snapshotID]; !exists {
				dstSyncList = append(dstSyncList, snapshotID)
			}
		}

		for _, snapshotID := range dstSyncList {
			err := synchronize(dstRepository, srcRepository, snapshotID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: could not synchronize snapshot %x from repository: %s\n", dstRepository.Location(), snapshotID, err)
			}
		}
	}
	return 0, nil
}

func synchronize(srcRepository *repository.Repository, dstRepository *repository.Repository, snapshotID objects.MAC) error {
	srcSnapshot, err := snapshot.Load(srcRepository, snapshotID)
	if err != nil {
		return err
	}
	defer srcSnapshot.Close()

	dstSnapshot, err := snapshot.New(dstRepository)
	if err != nil {
		return err
	}
	defer dstSnapshot.Close()

	// overwrite header, we want to keep the original snapshot info
	dstSnapshot.Header = srcSnapshot.Header

	iter, err := srcSnapshot.ListChunks()
	if err != nil {
		return err
	}
	for chunkID, err := range iter {
		if err != nil {
			return err
		}
		if !dstRepository.BlobExists(resources.RT_CHUNK, chunkID) {
			chunkData, err := srcSnapshot.GetBlob(resources.RT_CHUNK, chunkID)
			if err != nil {
				return err
			}
			dstSnapshot.PutBlob(resources.RT_CHUNK, chunkID, chunkData)
		}
	}

	iter, err = srcSnapshot.ListObjects()
	if err != nil {
		return err
	}
	for objectID, err := range iter {
		if err != nil {
			return err
		}
		if !dstRepository.BlobExists(resources.RT_OBJECT, objectID) {
			objectData, err := srcSnapshot.GetBlob(resources.RT_OBJECT, objectID)
			if err != nil {
				return err
			}
			dstSnapshot.PutBlob(resources.RT_OBJECT, objectID, objectData)
		}
	}

	fs, err := srcSnapshot.Filesystem()
	if err != nil {
		return err
	}

	iter, err = fs.FileMacs()
	if err != nil {
		return err
	}
	for entryID, err := range iter {
		if err != nil {
			return err
		}
		if !dstRepository.BlobExists(resources.RT_VFS_ENTRY, entryID) {
			entryData, err := srcSnapshot.GetBlob(resources.RT_VFS_ENTRY, entryID)
			if err != nil {
				return err
			}
			dstSnapshot.PutBlob(resources.RT_VFS_ENTRY, entryID, entryData)
		}
	}

	fsiter := fs.IterNodes()
	for fsiter.Next() {
		csum, node := fsiter.Current()
		if !dstRepository.BlobExists(resources.RT_VFS_BTREE, csum) {
			bytes, err := msgpack.Marshal(node)
			if err != nil {
				return err
			}
			dstSnapshot.PutBlob(resources.RT_VFS_BTREE, csum, bytes)
		}
	}
	if err := fsiter.Err(); err != nil {
		return err
	}

	return dstSnapshot.Commit()
}
