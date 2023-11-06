// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

func main() {
	var r [16]byte
	epochNow := time.Now().Unix()
	binary.BigEndian.PutUint32(r[0:4], uint32(epochNow))
	rand.Read(r[4:])
	fmt.Printf("%s", hex.EncodeToString(r[:]))
}
