package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

type Backend struct {
	TokenEndpoint         string `yaml:"token_endpoint"`
	IntrospectionEndpoint string `yaml:"introspection_endpoint"`
}

type Listener struct {
	ListenPort int `yaml:"port"`
}

type Config struct {
	Backend  Backend  `yaml:"backend"`
	Listener Listener `yaml:"listener"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	IdToken      string `json:"id_token"`
}

type ResponseBodyModifier func(body []byte) ([]byte, error)

func accessTokenToIdTokenResponseBodyModifier(body []byte) ([]byte, error) {
	// Parse the JSON Object
	var responseStruct TokenResponse
	err := json.Unmarshal(body, &responseStruct)
	if err != nil {
		return nil, err
	}
	// Copy the access_token to the id_token field and re-create the JSON structure
	responseStruct.IdToken = responseStruct.AccessToken
	responseRaw, err := json.Marshal(responseStruct)
	if err != nil {
		return nil, err
	}
	return responseRaw, nil
}

func proxyingPostHandlerWithResponseModifier(backendUrl string, responseBodyModifier ResponseBodyModifier, config *Config, debug *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			// Read the body from the request
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Error reading request body", http.StatusInternalServerError)
				return
			}
			// Forward the request to the backend
			backendReq, err := http.NewRequest("POST", backendUrl, bytes.NewReader(body))
			if err != nil {
				http.Error(w, "Error creating outbound request", http.StatusInternalServerError)
				return
			}
			// Copy headers from request to backend request
			backendReq.Header = r.Header
			// Send outbound request to backend
			client := &http.Client{}
			backendResponse, err := client.Do(backendReq)
			if err != nil {
				http.Error(w, "Error connecting to backend: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// Read the response body from the backend
			body, err = io.ReadAll(backendResponse.Body)
			if err != nil {
				http.Error(w, "Error reading response from backend: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// If the backend returned a non-success response, return it to the caller
			if backendResponse.StatusCode >= 300 || backendResponse.StatusCode < 200 {
				//http.Error(w, "Backend returned a non-success response: "+backendResponse.Status, backendResponse.StatusCode)
				_, err = w.Write(body)
				if err != nil {
					_, _ = fmt.Fprintln(os.Stderr, "Failure sending response to client: "+err.Error())
					return
				}
				return
			}

			// Debug
			if *debug {
				println("Debug enabled. Dumping response from backend:")
				println("Headers:")
				for k := range backendResponse.Header {
					println(k + ": " + strings.Join(backendResponse.Header[k], ","))
				}
				println("Body:")
				println(string(body))
			}
			// Run response body modifier, if defined
			var responseBody []byte
			if responseBodyModifier != nil {
				responseBody, err = responseBodyModifier(body)
				if err != nil {
					http.Error(w, "Error processing request body modifier: "+err.Error(), http.StatusInternalServerError)
					return
				}
			} else {
				responseBody = body
			}
			// Return response to initial request
			w.Header().Set("Content-Type", backendResponse.Header.Get("Content-Type"))
			_, err = w.Write(responseBody)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "Failure sending response to client: "+err.Error())
				return
			}
		} else {
			http.Error(w, "(proxyingPostHandlerWithResponseModifier) Invalid request method", http.StatusMethodNotAllowed)
		}
	}
}

func main() {
	// Read commandline parameter
	debug := flag.Bool("d", false, "Enable debug")
	flag.Parse()
	if *debug {
		println("Debug enabled")
	}
	// Read configuration
	content, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("failed to read the yaml file: %s", err)
	}
	config := Config{}
	err = yaml.Unmarshal(content, &config)
	if err != nil {
		log.Fatalf("failed to unmarshal the yaml file: %s", err)
	}
	http.HandleFunc("/token", proxyingPostHandlerWithResponseModifier(config.Backend.TokenEndpoint, accessTokenToIdTokenResponseBodyModifier, &config, debug))
	http.HandleFunc("/introspection", proxyingPostHandlerWithResponseModifier(config.Backend.IntrospectionEndpoint, nil, &config, debug))
	fmt.Printf("Starting server on port %d\n", config.Listener.ListenPort)
	err = http.ListenAndServe(fmt.Sprintf(":%d", config.Listener.ListenPort), nil)
	if err != nil {
		fmt.Println("ListenAndServe: ", err)
	}
}
