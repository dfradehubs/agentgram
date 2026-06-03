package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	codes   = sync.Map{}
	tokens  = sync.Map{}
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9999"
	}
	baseURL := "http://localhost:" + port

	mux := http.NewServeMux()

	mux.HandleFunc("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[metadata] discovery request from %s", r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"issuer":                 baseURL,
			"authorization_endpoint": baseURL + "/authorize",
			"token_endpoint":         baseURL + "/token",
			"registration_endpoint":  baseURL + "/register",
			"scopes_supported":       []string{"openid", "read", "write", "admin"},
			"response_types_supported": []string{"code"},
			"code_challenge_methods_supported": []string{"S256"},
		})
	})

	mux.HandleFunc("POST /register", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		clientName, _ := req["client_name"].(string)
		clientID := "dcr_" + generateRandom(8)
		log.Printf("[register] Dynamic Client Registration: client_name=%s → client_id=%s", clientName, clientID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"client_id":   clientID,
			"client_name": clientName,
		})
	})

	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		redirectURI := r.URL.Query().Get("redirect_uri")
		scope := r.URL.Query().Get("scope")
		clientID := r.URL.Query().Get("client_id")
		challenge := r.URL.Query().Get("code_challenge")

		log.Printf("[authorize] client_id=%s scope=%s challenge=%s", clientID, scope, challenge)

		code := generateRandom(16)
		codes.Store(code, map[string]string{
			"redirect_uri":   redirectURI,
			"code_challenge": challenge,
			"scope":          scope,
		})

		http.Redirect(w, r, fmt.Sprintf("%s?code=%s&state=%s", redirectURI, code, state), http.StatusFound)
	})

	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		grantType := r.FormValue("grant_type")
		log.Printf("[token] grant_type=%s", grantType)

		w.Header().Set("Content-Type", "application/json")

		switch grantType {
		case "authorization_code":
			code := r.FormValue("code")
			if _, ok := codes.Load(code); !ok {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			codes.Delete(code)

			accessToken := "at_" + generateRandom(32)
			refreshToken := "rt_" + generateRandom(32)
			tokens.Store(accessToken, true)

			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  accessToken,
				"refresh_token": refreshToken,
				"token_type":    "Bearer",
				"expires_in":    3600,
				"scope":         r.FormValue("scope"),
			})
			log.Printf("[token] issued access_token=%s...", accessToken[:20])

		case "refresh_token":
			accessToken := "at_" + generateRandom(32)
			tokens.Store(accessToken, true)

			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  accessToken,
				"token_type":    "Bearer",
				"expires_in":    3600,
			})
			log.Printf("[token] refreshed access_token=%s...", accessToken[:20])

		case "client_credentials":
			accessToken := "at_svc_" + generateRandom(32)
			tokens.Store(accessToken, true)

			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": accessToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
				"scope":        r.FormValue("scope"),
			})
			log.Printf("[token] service token issued")

		default:
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_grant_type"})
		}
	})

	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`, baseURL))
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		log.Printf("[mcp] request with auth=%s...", auth[:min(30, len(auth))])

		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		method, _ := req["method"].(string)
		id := req["id"]

		w.Header().Set("Content-Type", "application/json")

		switch method {
		case "initialize":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"protocolVersion": "2025-03-26",
					"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
					"serverInfo":      map[string]interface{}{"name": "mock-oauth2-mcp", "version": "1.0"},
				},
			})
			log.Printf("[mcp] initialized")

		case "notifications/initialized":
			w.WriteHeader(202)

		case "tools/list":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"tools": []interface{}{
						map[string]interface{}{
							"name":        "echo",
							"description": "Echoes the input back (OAuth2-protected tool)",
							"inputSchema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"message": map[string]interface{}{"type": "string"},
								},
								"required": []string{"message"},
							},
						},
					},
				},
			})

		case "tools/call":
			params, _ := req["params"].(map[string]interface{})
			args, _ := params["arguments"].(map[string]interface{})
			msg, _ := args["message"].(string)

			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]interface{}{
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": fmt.Sprintf("[OAuth2 MCP] Echo: %s (auth: %s...)", msg, auth[:min(20, len(auth))]),
						},
					},
				},
			})
			log.Printf("[mcp] tool call echo: %s", msg)

		default:
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"error":   map[string]interface{}{"code": -32601, "message": "method not found"},
			})
		}
	})

	mux.HandleFunc("GET /.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"resource":              baseURL,
			"authorization_servers": []string{baseURL},
		})
	})

	log.Printf("Mock OAuth2 MCP server on :%s", port)
	log.Printf("  Auth server:  %s/.well-known/oauth-authorization-server", baseURL)
	log.Printf("  MCP endpoint: %s/mcp", baseURL)
	log.Println("  Auto-approves all authorization requests (no login UI)")
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func generateRandom(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func min(a, b int) int {
	if a < b { return a }
	return b
}

func init() {
	_ = time.Now()
}
