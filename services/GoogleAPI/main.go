package GoogleAPI

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"superpose-sync/adapters/ConfigFile"
	"superpose-sync/utils"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
)

// Flags
var (
	clientID     = flag.String("clientid", "", "OAuth 2.0 Client ID.  If non-empty, overrides --clientid_file")
	clientIDFile = flag.String("clientid-file", "clientid.dat",
		"Name of a file containing just the project's OAuth 2.0 Client ID from https://developers.google.com/console.")
	secret     = flag.String("secret", "", "OAuth 2.0 Client Secret.  If non-empty, overrides --secret_file")
	secretFile = flag.String("secret-file", "clientsecret.dat",
		"Name of a file containing just the project's OAuth 2.0 Client Secret from https://developers.google.com/console.")
	cacheToken = flag.Bool("cachetoken", true, "cache the OAuth 2.0 token")
	debug      = flag.Bool("debug", false, "show HTTP traffic")
)

func getConfig() *oauth2.Config {
	*clientID = valueOrFileContents(*clientID, *clientIDFile)
	*secret = valueOrFileContents(*secret, *secretFile)
	config := &oauth2.Config{
		ClientID:     *clientID,
		ClientSecret: *secret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{drive.DriveScope, driveactivity.DriveActivityScope},
	}

	return config
}

func getContext() context.Context {
	ctx := context.Background()
	if *debug {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{
			Transport: &utils.LogTransport{http.DefaultTransport},
		})
	}
	return ctx
}

func newOAuthClient(ctx context.Context, config *oauth2.Config) *http.Client {
	token := getTokenConverted(config)
	if !token.Valid() {
		token = tokenFromWeb(ctx, config)
		ConfigFile.Configs.GoogleDrive.Token = ConfigFile.Token{
			AccessToken:  token.AccessToken,
			TokenType:    token.TokenType,
			RefreshToken: token.RefreshToken,
			Expiry:       token.Expiry,
			ExpiresIn:    int(token.Expiry.Sub(time.Now()).Seconds()),
			Scope:        strings.Join(config.Scopes, " "),
		}
		ConfigFile.SaveFile()
	}

	return config.Client(ctx, token)
}

func getTokenConverted(config *oauth2.Config) *oauth2.Token {
	token := &oauth2.Token{
		AccessToken:  ConfigFile.Configs.GoogleDrive.Token.AccessToken,
		TokenType:    ConfigFile.Configs.GoogleDrive.Token.TokenType,
		RefreshToken: ConfigFile.Configs.GoogleDrive.Token.RefreshToken,
		Expiry:       ConfigFile.Configs.GoogleDrive.Token.Expiry,
	}
	tokenSource := config.TokenSource(oauth2.NoContext, token)
	savedToken, err := tokenSource.Token()
	if err != nil {
		savedToken = token
	}
	return savedToken
}

func getOAuthClient() *http.Client {
	flag.Parse()

	if *clientID == "" {
		*clientID = ConfigFile.Configs.GoogleDrive.ClientId
		*secret = ConfigFile.Configs.GoogleDrive.ClientSecret
	}

	ctx := getContext()
	config := getConfig()
	c := newOAuthClient(ctx, config)
	return c
}

func tokenFromWeb(ctx context.Context, config *oauth2.Config) *oauth2.Token {
	ch := make(chan string)
	randState := fmt.Sprintf("st%d", time.Now().UnixNano())
	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/favicon.ico" {
			http.Error(rw, "", 404)
			return
		}
		if req.FormValue("state") != randState {
			log.Printf("State doesn't match: req = %#v", req)
			http.Error(rw, "", 500)
			return
		}
		if code := req.FormValue("code"); code != "" {
			fmt.Fprintf(rw, "<h1>Success</h1>Authorized.")
			rw.(http.Flusher).Flush()
			ch <- code
			return
		}
		log.Printf("no code")
		http.Error(rw, "", 500)
	}))
	defer ts.Close()

	config.RedirectURL = ts.URL
	authURL := config.AuthCodeURL(randState)
	go openURL(authURL)
	log.Printf("Authorize this app at: %s", authURL)
	code := <-ch
	log.Printf("Got code: %s", code)

	token, err := config.Exchange(ctx, code)
	if err != nil {
		log.Fatalf("Token exchange error: %v", err)
	}
	return token
}

func openURL(url string) {
	try := []string{"xdg-open", "google-chrome", "open"}
	for _, bin := range try {
		err := exec.Command(bin, url).Run()
		if err == nil {
			return
		}
	}
	log.Printf("Error opening URL in browser.")
}

func valueOrFileContents(value string, filename string) string {
	if value != "" {
		return value
	}
	slurp, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("Error reading %q: %v", filename, err)
	}
	return strings.TrimSpace(string(slurp))
}

func NewDrive() GoogleDrive {
	return NewService()
}
