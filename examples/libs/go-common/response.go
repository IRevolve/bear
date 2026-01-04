package gocommon




























}	json.NewEncoder(w).Encode(Response{Success: false, Error: message})	w.WriteHeader(status)	w.Header().Set("Content-Type", "application/json")func Error(w http.ResponseWriter, status int, message string) {// Error writes an error response.}	json.NewEncoder(w).Encode(Response{Success: status < 400, Data: data})	w.WriteHeader(status)	w.Header().Set("Content-Type", "application/json")func JSON(w http.ResponseWriter, status int, data interface{}) {// JSON writes a JSON response.}	Error   string      `json:"error,omitempty"`	Data    interface{} `json:"data,omitempty"`	Success bool        `json:"success"`type Response struct {// Response is a standard API response wrapper.)	"net/http"	"encoding/json"import (package common// Package common provides shared utilities for all Go services.// test comment
// test
