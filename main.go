package main

import (
	"encoding/json"
	"fmt"
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
	"google.golang.org/api/drive/v3"
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
		url.QueryEscape("drive-go-quickstart.json")), err
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


func main() {
	ctx := context.Background()

	b, err := ioutil.ReadFile("client_secret.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved credentials
	// at ~/.credentials/drive-go-quickstart.json
	config, err := google.ConfigFromJSON(b, drive.DriveMetadataReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(ctx, config)

	srv, err := drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve drive Client %v", err)
	}

	// Get file list
	// files := make(map[string]*drive.File) // key: File.Md5Checksum
	files := []*drive.File{}
	// Read files from cache if available
	if _, err := os.Stat("files.json"); err == nil {
		fmt.Printf("Read files.json\n")
		filesJson, err := ioutil.ReadFile("files.json")
		if err != nil {
			log.Fatalf("ioutil.ReadFile(files.json) failed: %v", err)
		}
		// files := map[string]*drive.File
		err = json.Unmarshal(filesJson, &files)
		if err != nil {
			log.Fatalf("json.Unmarshal(filesJson) failed: %v", err)
		}
	}

  // Read from remote
	if len(files) == 0 {
		var numFiles int
		var pageToken string
		for {
			list := srv.Files.List().
				PageSize(1000).
				// Q("not mimeType contains 'application/vnd.google-apps'").
				Fields("nextPageToken, files(id, name, md5Checksum, mimeType, parents)")
			if pageToken != "" {
				list = list.PageToken(pageToken)
			}
			r, err := list.Do()
			if err != nil {
				log.Fatalf("Unable to retrieve files: %v", err)
			}
			numFiles += len(r.Files)
			for _, i := range r.Files {
				fmt.Printf("%s (md5: %s, type: %s, id: %s, parents: %v)\n", i.Name, i.Md5Checksum, i.MimeType, i.Id, i.Parents)
				// if i.Md5Checksum != "" {
				// 	files[i.Md5Checksum] = i
				// }
				files = append(files, i)
			}
			fmt.Printf("count:%d\n\n", numFiles)
			if r.NextPageToken == "" {
				break
			}
			pageToken = r.NextPageToken
			// break //DEBUG
		}
	}

	// Cache files
	fmt.Printf("Write files.json\n")
	filesJson , err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		log.Fatalf("json.Marshal(files) failed: %v", err)
	}
	err = ioutil.WriteFile("files.json", filesJson, 0644)
	if err != nil {
		log.Fatalf("ioutil.WriteFile(fileJson) failed: %v", err)
	}

	// Extract folders
	folders := make(map[string]*drive.File) // key: File.Id
	for _, file := range files {
		if file.MimeType == "application/vnd.google-apps.folder" {
			folders[file.Id] = file
		}
	}
  fmt.Printf("folders: %d\n", len(folders))

	// debug: print path of real files
	for _, file := range files {
		if file.Md5Checksum != "" {
			// path := file.Name
			// folder := folders[file.Parents[0]]
			path := ""
			for file != nil {
				path = "/" + file.Name + path
				// fmt.Printf("folder: %+v\n", folder)
				if file.Parents != nil {
					f, _ := folders[file.Parents[0]]
					file = f
				} else {
					file = nil
				}
			}
			fmt.Printf("%s\n", path)
		}
	}
}
