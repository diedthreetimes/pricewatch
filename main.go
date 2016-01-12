package main

import (
  "encoding/json"
  "encoding/base64"
  "fmt"
  "bytes"
  "io/ioutil"
  "log"
  "net/http"
  "net/url"
  "os"
  "os/user"
  "path/filepath"

  "golang.org/x/net/context"
  "golang.org/x/oauth2"
  "golang.org/x/oauth2/google"
  "google.golang.org/api/gmail/v1"
  
  "github.com/PuerkitoBio/goquery"
)

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
  cacheFile, err := tokenCacheFile()
  if err != nil {
    log.Fatalf("Unable to get path to cached credential file. %v", err)
  }
  tok, err := tokenFromFile(cacheFile)
  if err != nil {
    tok = getTokenFromWeb(config)
    saveToken(cacheFile, tok)
  }
  return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
  authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
  fmt.Printf("Go to the following link in your browser then type the "+
    "authorization code: \n%v\n", authURL)

  var code string
  if _, err := fmt.Scan(&code); err != nil {
    log.Fatalf("Unable to read authorization code %v", err)
  }

  tok, err := config.Exchange(oauth2.NoContext, code)
  if err != nil {
    log.Fatalf("Unable to retrieve token from web %v", err)
  }
  return tok
}

// tokenCacheFile generates credential file path/filename.
// It returns the generated credential path/filename.
func tokenCacheFile() (string, error) {
  usr, err := user.Current()
  if err != nil {
    return "", err
  }
  tokenCacheDir := filepath.Join(usr.HomeDir, ".credentials")
  os.MkdirAll(tokenCacheDir, 0700)
	return filepath.Join(tokenCacheDir,
		url.QueryEscape("gmail-go-quickstart.json")), err // TODO: Change this
}

// tokenFromFile retrieves a Token from a given file path.
// It returns the retrieved Token and any read error encountered.
func tokenFromFile(file string) (*oauth2.Token, error) {
  f, err := os.Open(file)
  if err != nil {
    return nil, err
  }
  t := &oauth2.Token{}
  err = json.NewDecoder(f).Decode(t)
  defer f.Close()
  return t, err
}

// saveToken uses a file path to create a file and store the
// token in it.
func saveToken(file string, token *oauth2.Token) {
  fmt.Printf("Saving credential file to: %s\n", file)
  f, err := os.Create(file)
  if err != nil {
    log.Fatalf("Unable to cache oauth token: %v", err)
  }
  defer f.Close()
  json.NewEncoder(f).Encode(token)
}

// TODO: Make this dependent on the current date
// TODO: Make this adjustable based on the CC requirements
func amazonQueryString() (string) {
  return "from:auto-confirm@amazon.com after:2015/12/11 before:2016/2/12"
}

func getOrders(encodedEmail string) ([]string, error) {
  ret := make([]string, 0, 5)

  data, err := base64.URLEncoding.DecodeString(encodedEmail)
  
  if err != nil {
    return nil, err
  }

//  log.Printf("This is it %q\n", data)
  // append (ret, item)

  doc, err := goquery.NewDocumentFromReader(bytes.NewBuffer(data))
  if err != nil {
    return nil, err
  }
  
  doc.Find("div.greeting ~ a").Each( func(i int, s *goquery.Selection) {
    link, _ := s.Attr("href")
    ret = append(ret, link)
  })

  return ret, nil
}

func main() {
  ctx := context.Background()

  b, err := ioutil.ReadFile("client_secret.json")
  if err != nil {
    log.Fatalf("Unable to read client secret file: %v", err)
  }

  config, err := google.ConfigFromJSON(b, gmail.GmailReadonlyScope)
  if err != nil {
    log.Fatalf("Unable to parse client secret file to config: %v", err)
  }
  client := getClient(ctx, config)

  srv, err := gmail.New(client)
  if err != nil {
    log.Fatalf("Unable to retrieve gmail Client %v", err)
  }

  user := "me"

  pageToken := ""
  for {
    req := srv.Users.Messages.List(user).Q(amazonQueryString())
    if pageToken != "" {
      req.PageToken(pageToken)
    }

    r, err := req.Do()
    if err != nil {
      log.Fatalf("Unable to retrieve messages. %v", err)
    }

    log.Printf("Processing %v messages...\n", len(r.Messages))
    for _, m := range r.Messages {

      msg, err := srv.Users.Messages.Get(user, m.Id).Do()
      if err != nil {
        log.Fatalf("Unable to retrieve message %v: %v", m.Id, err)
      }
      
      // TODO: Make these fatals just log
      // TODO: This is fragile and the structure could easily change, should build something more sophisticated (like just a function :P)
      // TODO: Use a queue in order to support multipart containers with multiple multipart containers or various other nesting strategies (and potentially images?)

      part := msg.Payload

      // Esentially we are just checking and traversing for multipart/mixed => multipart/alternative => text/html
      // At the end part = the version of the email with html
      if part.MimeType == "multipart/mixed" {
        for _, p := range part.Parts {
          if p.MimeType == "multipart/alternative" {
            part = p

            for _, p := range part.Parts {
              if p.MimeType == "text/html" {
                part = p                
                break
              } 
            }
            break
          } 
        }
        
        if part.MimeType != "text/html" {
          log.Fatalf("mixed message %v didn't have a nested html type", m.Id)
        } 
      } else {
        log.Fatalf("No multipart/mixed in message %v", m.Id)
      }
             
      items, err := getOrders(part.Body.Data)

      if err != nil {
        log.Fatalf("getOrders failed %v", err)
      }

      for _, item := range items {
        // TODO: Do something useful with the order links (maybe just collect them and store in a DB)?
        // We ultimately need to turn these orders into specific items!
        log.Printf("You bought %v\n", item)
      }
    }

    if r.NextPageToken == "" {
      break
    }
    pageToken = r.NextPageToken
  }
}
