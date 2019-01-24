package main

import (
	"context"
	"errors"

	"github.com/kmeaw/desync"
	"github.com/spf13/cobra"
)

type chopOptions struct {
	cmdStoreOptions
	store         string
	ignoreIndexes []string
}

func newChopCommand(ctx context.Context) *cobra.Command {
	var opt chopOptions

	cmd := &cobra.Command{
		Use:   "chop <index> <file>",
		Short: "Reads chunks from a file according to an index",
		Long: `Reads the index and extracts all referenced chunks from the file into a store,
local or remote.

Does not modify the input file or index in any. It's used to populate a chunk
store by chopping up a file according to an existing index.

Use '-' to read the index from STDIN.`,
		Example: `  desync chop -s sftp://192.168.1.1/store file.caibx largefile.bin`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChop(ctx, opt, args)
		},
		SilenceUsage: true,
	}
	flags := cmd.Flags()
	flags.StringVarP(&opt.store, "store", "s", "", "target store")
	flags.StringSliceVarP(&opt.ignoreIndexes, "ignore", "", nil, "index(s) to ignore chunks from")
	flags.IntVarP(&opt.n, "concurrency", "n", 10, "number of concurrent goroutines")
	flags.BoolVarP(&desync.TrustInsecure, "trust-insecure", "t", false, "trust invalid certificates")
	flags.StringVar(&opt.clientCert, "client-cert", "", "path to client certificate for TLS authentication")
	flags.StringVar(&opt.clientKey, "client-key", "", "path to client key for TLS authentication")
	return cmd
}

func runChop(ctx context.Context, opt chopOptions, args []string) error {
	if (opt.clientKey == "") != (opt.clientCert == "") {
		return errors.New("--client-key and --client-cert options need to be provided together")
	}
	if opt.store == "" {
		return errors.New("no target store provided")
	}

	indexFile := args[0]
	dataFile := args[1]

	// Open the target store
	s, err := WritableStore(opt.store, opt.cmdStoreOptions)
	if err != nil {
		return err
	}
	defer s.Close()

	// Read the input
	c, err := readCaibxFile(indexFile, opt.cmdStoreOptions)
	if err != nil {
		return err
	}
	chunks := c.Chunks

	// If requested, skip/ignore all chunks that are referenced in other indexes
	if len(opt.ignoreIndexes) > 0 {
		m := make(map[desync.ChunkID]desync.IndexChunk)
		for _, c := range chunks {
			m[c.ID] = c
		}
		for _, f := range opt.ignoreIndexes {
			i, err := readCaibxFile(f, opt.cmdStoreOptions)
			if err != nil {
				return err
			}
			for _, c := range i.Chunks {
				delete(m, c.ID)
			}
		}
		chunks = make([]desync.IndexChunk, 0, len(m))
		for _, c := range m {
			chunks = append(chunks, c)
		}
	}

	// If this is a terminal, we want a progress bar
	pb := NewProgressBar("")

	// Chop up the file into chunks and store them in the target store
	return desync.ChopFile(ctx, dataFile, chunks, s, opt.n, pb)
}
