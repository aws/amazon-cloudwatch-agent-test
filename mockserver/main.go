// Copyright 2023 Amazon.com, Inc. or its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"path"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
)

const (
	HealthCheckMessage = "healthcheck"
	SuccessMessage     = "success"
)

var (
	CertFilePath = path.Join("certificates", "ssl", "certificate.crt")
	KeyFilePath  = path.Join("certificates", "private.key")
)

type transactionHttpServer struct {
	transactions uint32
	startTime    time.Time
}

type TransactionPayload struct {
	TransactionsPerMinute float64 `json:"GetNumberOfTransactionsPerMinute"`
}

func healthCheck(w http.ResponseWriter, _ *http.Request) {
	if _, err := io.WriteString(w, HealthCheckMessage); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("Unable to write response: %v", err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (ts *transactionHttpServer) checkTransactionCount(w http.ResponseWriter, _ *http.Request) {
	var message string
	var t = atomic.LoadUint32(&ts.transactions)
	if t > 0 {
		message = SuccessMessage
	}
	log.Printf("\033[31m Time: %d | checkTransactionCount msg: %s | %d\033[0m \n", time.Now().Unix(), message, t)
	if _, err := io.WriteString(w, message); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, err.Error())
		log.Printf("Unable to write response: %v", err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (ts *transactionHttpServer) recordTransaction(w http.ResponseWriter, _ *http.Request) {
	atomic.AddUint32(&ts.transactions, 1)

	// Built-in latency
	log.Printf("\033[31m Time: %s | transaction received \033[0m \n", time.Now().String())
	time.Sleep(15 * time.Millisecond)
	w.WriteHeader(http.StatusOK)
}

// Retrieve number of transactions per minute
func (ts *transactionHttpServer) GetNumberOfTransactionsPerMinute(w http.ResponseWriter, _ *http.Request) {
	// Calculate duration in minutes
	duration := time.Now().Sub(ts.startTime)
	transactions := float64(atomic.LoadUint32(&ts.transactions))
	tpm := transactions / duration.Minutes()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(TransactionPayload{tpm}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, err.Error())
		log.Printf("Unable to write response: %v", err)
	}
}

// Starts an HTTP server that receives request from validator only to verify the data ingestion
func StartHttpServer() {
	var wg sync.WaitGroup
	log.Println("\033[31m Starting Server \033[0m")
	store := transactionHttpServer{startTime: time.Now()}
	//2 servers one for receiving the data , one for verify data
	dataApp := mux.NewRouter()
	dataReceiverServer := &http.Server{Addr: ":443", Handler: dataApp}
	verificationRequestServer := http.NewServeMux()
	appServer := &http.Server{Addr: ":8080", Handler: verificationRequestServer}
	wg.Add(2)
	go func(ts *transactionHttpServer) {
		defer wg.Done()
		dataApp.HandleFunc("/ping", healthCheck)
		dataApp.PathPrefix("/put-data").HandlerFunc(ts.recordTransaction)
		dataApp.HandleFunc("/trace/v1", ts.recordTransaction)
		dataApp.HandleFunc("/metric/v1", ts.recordTransaction)
		if err := dataReceiverServer.ListenAndServeTLS(CertFilePath, KeyFilePath); err != nil {
			log.Printf("HTTPS server error: %v", err)
			err = dataReceiverServer.Shutdown(context.TODO())
			log.Fatalf("Shutdown server error: %v", err)
		}
	}(&store)

	go func(ts *transactionHttpServer) {
		defer wg.Done()
		verificationRequestServer.HandleFunc("/ping", healthCheck)
		verificationRequestServer.HandleFunc("/check-data", ts.checkTransactionCount)
		verificationRequestServer.HandleFunc("/tpm", ts.GetNumberOfTransactionsPerMinute)
		if err := appServer.ListenAndServe(); err != nil {
			log.Printf("Verification server error: %v", err)
			err := appServer.Shutdown(context.TODO())
			log.Fatalf("Shutdown server error: %v", err)
		}
	}(&store)
	wg.Wait()
	log.Println("\033[32m Stopping Server \033[0m")
}

func main() {
	StartHttpServer()
}
