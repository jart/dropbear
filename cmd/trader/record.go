package main

import (
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/clocky"
	"dropbear/gcs"
	"encoding/binary"
	"fmt"
	"log"
	"unsafe"

	"github.com/klauspost/compress/zstd"
)

const messageSize = int(unsafe.Sizeof(sip.Message{}))

func recordTape(now clocky.Time, ch <-chan alpaca.StockUpdate, done chan struct{}) {
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

	// write magic
	magicBuffer := [4]byte{128 + 2, 'S', 'I', 'P'} // SIP v2 file format
	if _, err := zw.Write(magicBuffer[:]); err != nil {
		panic(fmt.Sprintf("tape: write error: %v", err))
	}

	// write messages as raw 56-byte structs
	var n int64
	var tsBuffer [8]byte
	for stockUpdate := range ch {
		// write stockUpdate.ReceivedAt as little endian 64-bit word
		binary.LittleEndian.PutUint64(tsBuffer[:], uint64(stockUpdate.ReceivedAt))
		if _, err := zw.Write(tsBuffer[:]); err != nil {
			log.Printf("tape: write error: %v", err)
			break
		}
		msgBuffer := (*[messageSize]byte)(unsafe.Pointer(&stockUpdate.Message))
		if _, err := zw.Write(msgBuffer[:]); err != nil {
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
