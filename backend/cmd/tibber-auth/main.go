// Command tibber-auth obtains the OAuth2 refresh token the tibber.prices widget
// needs. Tibber only issues tokens after a login in the browser, so run this
// once on a machine with a browser:
//
//	TIBBER_CLIENT_ID=... TIBBER_CLIENT_SECRET=... go run ./cmd/tibber-auth
//
// The redirect URI registered for the client must be http://localhost:8080/
// (change it with -redirect if you registered another local one). Stop Lumina
// while this runs if it listens on the same port.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	authorizeURL = "https://thewall.tibber.com/connect/authorize"
	tokenURL     = "https://thewall.tibber.com/connect/token"
	scopes       = "openid profile email offline_access data-api-user-read data-api-homes-read data-api-prices-read"
)

func main() {
	redirect := flag.String("redirect", "http://localhost:8080/", "redirect URI registered for the OAuth client")
	flag.Parse()
	id, secret := os.Getenv("TIBBER_CLIENT_ID"), os.Getenv("TIBBER_CLIENT_SECRET")
	if id == "" || secret == "" {
		fmt.Fprintln(os.Stderr, "set TIBBER_CLIENT_ID and TIBBER_CLIENT_SECRET")
		os.Exit(2)
	}
	ru, err := url.Parse(*redirect)
	if err != nil || ru.Host == "" {
		fmt.Fprintln(os.Stderr, "bad -redirect")
		os.Exit(2)
	}
	state := randomHex()

	codes := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(ru.Path, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("error") != "":
			http.Error(w, "Tibber: "+q.Get("error")+" "+q.Get("error_description"), http.StatusBadRequest)
			fmt.Fprintln(os.Stderr, "authorization failed:", q.Get("error"), q.Get("error_description"))
			os.Exit(1)
		case q.Get("state") != state || q.Get("code") == "":
			http.Error(w, "unexpected request", http.StatusBadRequest)
		default:
			fmt.Fprintln(w, "Done. You can close this tab and return to the terminal.")
			select {
			case codes <- q.Get("code"):
			default:
			}
		}
	})
	ln, err := net.Listen("tcp", ru.Host)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot listen on", ru.Host, "(is Lumina running there?):", err)
		os.Exit(1)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())

	q := url.Values{"response_type": {"code"}, "client_id": {id}, "redirect_uri": {*redirect}, "scope": {scopes}, "state": {state}}
	fmt.Println("Open this URL in your browser and log in with your Tibber account:\n\n  " + authorizeURL + "?" + q.Encode() + "\n")

	var code string
	select {
	case code = <-codes:
	case <-time.After(10 * time.Minute):
		fmt.Fprintln(os.Stderr, "timed out waiting for the login")
		os.Exit(1)
	}

	resp, err := http.PostForm(tokenURL, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {*redirect},
		"client_id": {id}, "client_secret": {secret},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr struct {
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
	}
	_ = json.Unmarshal(body, &tr)
	if resp.StatusCode/100 != 2 || tr.RefreshToken == "" {
		fmt.Fprintf(os.Stderr, "token exchange failed (HTTP %d): %s\n", resp.StatusCode, tr.Error)
		os.Exit(1)
	}
	fmt.Println("Success. Put the refresh token into the environment Lumina runs in:")
	fmt.Println("\n  TIBBER_REFRESH_TOKEN=" + tr.RefreshToken + "\n")
	if tr.Scope != "" && !strings.Contains(tr.Scope, "data-api-prices-read") {
		fmt.Println("WARNING: the granted scopes do not include data-api-prices-read:", tr.Scope)
	}
}

func randomHex() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
