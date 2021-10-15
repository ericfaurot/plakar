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

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"strings"

	"github.com/poolpOrg/plakar"
	"github.com/poolpOrg/plakar/encryption"
	"github.com/poolpOrg/plakar/helpers"
	"github.com/poolpOrg/plakar/local"
	"github.com/poolpOrg/plakar/storage"
	"github.com/poolpOrg/plakar/storage/client"
	"github.com/poolpOrg/plakar/storage/fs"
)

var localdir string
var hostname string
var storeloc string
var nocache bool
var outputLog string

const VERSION = "0.0.1"

func keypairGenerate() ([]byte, error) {
	keypair, err := encryption.Keygen()
	if err != nil {
		return nil, err
	}

	var passphrase []byte
	for {
		passphrase, err = helpers.GetPassphraseConfirm()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			continue
		}
		break
	}

	pem, err := keypair.Encrypt(passphrase)
	if err != nil {
		return nil, err
	}

	return pem, err
}

func clearline(length int) {
	buf := make([]byte, length)
	for i := 0; i < length; i++ {
		buf[i] = byte(' ')
	}
	fmt.Fprintf(os.Stdin, "\r%s", string(buf))
}

func main() {
	ctx := plakar.Plakar{}

	hostbuf, err := os.Hostname()
	if err != nil {
		hostbuf = "localhost"
	}

	pwUser, err := user.Current()
	if err != nil {
		log.Fatalf("%s: user %s has turned into Casper", flag.CommandLine.Name(), pwUser.Username)
	}

	flag.StringVar(&localdir, "local", fmt.Sprintf("%s/.plakar", pwUser.HomeDir), "local store")
	flag.StringVar(&hostname, "hostname", strings.ToLower(hostbuf), "local hostname")
	flag.StringVar(&outputLog, "log", "", "show progress")

	if outputLog != "" && outputLog != "full" && outputLog != "progress" {
		flag.Usage()
		os.Exit(1)
	}

	flag.Parse()
	if len(flag.Args()) == 0 {
		log.Fatalf("%s: missing command", flag.CommandLine.Name())
	}

	command, args := flag.Arg(0), flag.Args()[1:]
	storeloc = fmt.Sprintf("%s/store", localdir)
	if flag.Arg(0) == "on" {
		if len(flag.Args()) < 2 {
			log.Fatalf("%s: missing plakar repository", flag.CommandLine.Name())
		}
		if len(flag.Args()) < 3 {
			log.Fatalf("%s: missing command", flag.CommandLine.Name())
		}
		storeloc = flag.Arg(1)
		command, args = flag.Arg(2), flag.Args()[3:]
	}

	/* PlakarCTX */
	ctx.Localdir = localdir
	ctx.Hostname = strings.ToLower(hostname)
	ctx.Username = pwUser.Username

	doneChannel := make(chan bool)
	ctx.StdoutChannel = make(chan interface{})
	ctx.StderrChannel = make(chan interface{})
	go func() {
		linesize := 0
		for {
			select {
			case msg, more := <-ctx.StdoutChannel:
				if !more {
					doneChannel <- true
					return
				}
				if outputLog == "progress" {
					clearline(linesize)
					fmt.Printf("\r%s", msg)
					if len(msg.(string)) > linesize {
						linesize = len(msg.(string))
					}
				}
				if outputLog == "full" {
					fmt.Printf("%s\n", msg)
				}

			case msg := <-ctx.StderrChannel:
				if outputLog == "progress" {
					clearline(linesize)
					fmt.Fprintf(os.Stderr, "\r%s\n", msg)
					if len(msg.(string)) > linesize {
						linesize = len(msg.(string))
					}
				} else {
					fmt.Fprintf(os.Stderr, "%s\n", msg)
				}
			}
		}
	}()

	/* first thing first, initialize a plakar local if none */
	local.Init(localdir)
	if !nocache {
		ctx.Cache = &local.Cache{}
	}

	/* keygen command needs to be handled very early */
	if command == "keygen" {
		err = cmd_keygen(ctx, args)
		if err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	/* load keypair from plakar */
	encryptedKeypair, err := local.GetEncryptedKeypair(localdir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "key not found, run `plakar keygen`\n")
			os.Exit(1)
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			os.Exit(1)
		}
	}
	ctx.EncryptedKeypair = encryptedKeypair

	// create command needs to be handled early _after_ key is available
	if command == "create" {
		cmd_create(ctx, args)
		if err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	var store storage.Store
	if strings.HasPrefix(storeloc, "plakar://") {
		pstore := &client.ClientStore{}
		pstore.Ctx = &ctx
		pstore.Repository = storeloc
		store = pstore

	} else {
		pstore := &fs.FSStore{}
		pstore.Ctx = &ctx
		pstore.Repository = storeloc
		store = pstore
	}

	store.Init()
	err = store.Open()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "store does not seem to exist: run `plakar init`\n")
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", err)
		}
		return
	}

	if store.Configuration().Encrypted != "" {
		var keypair *encryption.Keypair
		for {
			passphrase, err := helpers.GetPassphrase()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}

			keypair, err = encryption.Keyload(passphrase, encryptedKeypair)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err)
				continue
			}
			break
		}
		ctx.Keypair = keypair
	}

	switch command {

	case "cat":
		cmd_cat(store, args)

	case "check":
		cmd_check(store, args)

	case "diff":
		cmd_diff(store, args)

	case "find":
		cmd_find(store, args)

	case "info":
		cmd_info(store, args)

	case "key":
		cmd_key(store, args)

	case "ls":
		cmd_ls(store, args)

	case "pull":
		cmd_pull(store, args)

	case "push":
		cmd_push(store, args)

	case "rm":
		cmd_rm(store, args)

	case "server":
		cmd_server(store, args)

	case "tarball":
		cmd_tarball(store, args)

	case "ui":
		cmd_ui(store, args)

	case "version":
		cmd_version(store, args)

	default:
		log.Fatalf("%s: unsupported command: %s", flag.CommandLine.Name(), command)
	}
	close(ctx.StdoutChannel)
	<-doneChannel
}
