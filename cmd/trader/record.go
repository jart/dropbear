package main

import (
	"bufio"
	"context"
	"dropbear/broker/alpaca/sip"
	"dropbear/clocky"
	"fmt"
	"log"
	"os"
	"unsafe"

	"cloud.google.com/go/storage"
	"github.com/klauspost/compress/zstd"
)

const messageSize = int(unsafe.Sizeof(sip.Message{}))

func recordTape(ch <-chan sip.Message, done chan struct{}) {
	defer close(done)

	// connect to gcs
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Printf("tape: error creating gcs client: %v", err)
		os.Exit(1)
	}
	defer client.Close()

	// create object named by today's date
	name := fmt.Sprintf("%s.sip.zst", clocky.Now())
	obj := client.Bucket(*flagBucket).Object(name).NewWriter(ctx)
	obj.ContentType = "application/zstd"
	log.Printf("tape: recording to gs://%s/%s", *flagBucket, name)

	// stack: channel -> bufio -> zstd -> gcs
	zw, err := zstd.NewWriter(obj)
	if err != nil {
		log.Printf("tape: error creating zstd writer: %v", err)
		os.Exit(1)
	}
	buf := bufio.NewWriterSize(zw, 256*1024)

	// write messages as raw 56-byte structs
	var n int64
	for msg := range ch {
		b := (*[messageSize]byte)(unsafe.Pointer(&msg))
		if _, err := buf.Write(b[:]); err != nil {
			log.Printf("tape: write error: %v", err)
			break
		}
		n++
	}

	// flush everything
	if err := buf.Flush(); err != nil {
		log.Printf("tape: flush error: %v", err)
	}
	if err := zw.Close(); err != nil {
		log.Printf("tape: zstd close error: %v", err)
	}
	if err := obj.Close(); err != nil {
		log.Printf("tape: gcs close error: %v", err)
	}
	log.Printf("tape: recorded %d messages to gs://%s/%s", n, *flagBucket, name)
}

func drainChannel(ch <-chan sip.Message) {
	for range ch {
	}
}
