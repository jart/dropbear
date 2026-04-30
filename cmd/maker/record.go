package main

import (
	"dropbear/broker/alpaca/sip"
	"dropbear/clocky"
	"dropbear/gcs"
	"fmt"
	"log"
	"unsafe"

	"github.com/klauspost/compress/zstd"
)

const messageSize = int(unsafe.Sizeof(sip.Message{}))

func recordTape(now clocky.Time, ch <-chan *sip.Message, done chan struct{}) {
	defer close(done)

	// stream to gcs
	name := fmt.Sprintf("%s.sip.zst", now)
	obj, err := gcs.NewWriter(*flagBucket, name)
	if err != nil {
		panic(fmt.Sprintf("tape: error creating gcs writer: %v", err))
	}
	log.Printf("tape: recording to gs://%s/%s", *flagBucket, name)
	defer obj.Close()

	// compress file
	zw, err := zstd.NewWriter(obj)
	if err != nil {
		panic(fmt.Sprintf("tape: error creating zstd writer: %v", err))
	}
	defer zw.Close()

	// write messages as raw 56-byte structs
	var n int64
	for msg := range ch {
		b := (*[messageSize]byte)(unsafe.Pointer(msg))
		if _, err := zw.Write(b[:]); err != nil {
			log.Printf("tape: write error: %v", err)
			break
		}
		n++
	}

	// close output
	if err := zw.Close(); err != nil {
		log.Printf("tape: zstd close error: %v", err)
	}
	if err := obj.Close(); err != nil {
		log.Printf("tape: gcs close error: %v", err)
	}
	log.Printf("tape: recorded %d messages to gs://%s/%s", n, *flagBucket, name)
}

func recordLog(now clocky.Time, ch <-chan string, done chan struct{}) {
	defer close(done)

	// stream to gcs
	name := fmt.Sprintf("%s.log.zst", now)
	obj, err := gcs.NewWriter(*flagBucket, name)
	if err != nil {
		panic(fmt.Sprintf("tape: error creating gcs writer: %v", err))
	}
	log.Printf("tape: recording to gs://%s/%s", *flagBucket, name)
	defer obj.Close()

	// compress file
	zw, err := zstd.NewWriter(obj)
	if err != nil {
		panic(fmt.Sprintf("tape: error creating zstd writer: %v", err))
	}
	defer zw.Close()

	// write lines
	var n int64
	for msg := range ch {
		b := []byte(msg)
		if _, err := zw.Write(b); err != nil {
			log.Printf("tape: write error: %v", err)
			break
		}
		n++
	}

	// close output
	if err := zw.Close(); err != nil {
		log.Printf("tape: zstd close error: %v", err)
	}
	if err := obj.Close(); err != nil {
		log.Printf("tape: gcs close error: %v", err)
	}
	log.Printf("tape: recorded %d messages to gs://%s/%s", n, *flagBucket, name)
}
