package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	// "errors"
	"fmt"
	"io"
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


func driveService() *drive.Service {
	b, err := ioutil.ReadFile("client_secret.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}
	// If modifying these scopes, delete your previously saved credentials
	// at ~/.credentials/drive-go-quickstart.json
	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(context.Background(), config)

	srv, err := drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve drive Client %v", err)
	}
	return srv
}

// Read from remote
func remote(srv *drive.Service) []drive.File {
	var files []drive.File
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
			files = append(files, *i)
		}
		fmt.Printf("count:%d\n\n", numFiles)
		if r.NextPageToken == "" {
			break
		}
		pageToken = r.NextPageToken
	}
	return files
}

func local(basePath string) []localFile {
	var files []localFile
  walkFunc := func(path string, f os.FileInfo, err error) error {
		// fmt.Printf("%s (%+v)\n", path, f)
		if err != nil {
			log.Printf("walkFunc(%s) with error: %v", path, err)
		}
		// fmt.Printf("%s (%+v)\n", path, f)
		if f.IsDir() {
			return nil
		}
		// fmt.Printf("f.Sys() => %+v", f.Sys())
		b, err := ioutil.ReadFile(path)
		if err != nil {
			log.Fatalf("ioutil.ReadFile(%s) failed %v", path, err)
		}
		md5sum := md5.Sum(b)
		md5hex := hex.EncodeToString(md5sum[:])
		relativePath, _ := filepath.Rel(basePath, path)
		fmt.Printf("%s (md5: %s)\n", relativePath, md5hex)
		files = append(files, localFile{relativePath, md5hex})
		// return errors.New("stop")
		return nil
	}

	err := filepath.Walk(basePath, walkFunc)
	if err != nil && err.Error() != "stop" {
		log.Fatalf("filepath.Walk(%s) failed: %v", basePath, err)
	}
	// fmt.Printf("files:%v", files)
	return files
}

type localFile struct {
	Path string
	Md5Checksum string
}

type Files struct {
	Remote []drive.File
	Local []localFile
}

func remotePath(folders map[string]drive.File, file drive.File) string {
	f := &file
	path := ""
	for f != nil {
		path = "/" + f.Name + path
		if f.Parents != nil {
			d, ok := folders[f.Parents[0]]
			if ok {
				f = &d
			} else {
				f = nil
			}
		} else {
			f = nil
		}
	}
	return path
}

func readFilesJson() *Files {
	var files Files
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
	return &files
}

func writeFilesJson(files *Files) {
	filesJson , err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		log.Fatalf("json.Marshal(files) failed: %v", err)
	}
	err = ioutil.WriteFile("files.json", filesJson, 0644)
	if err != nil {
		log.Fatalf("ioutil.WriteFile(fileJson) failed: %v", err)
	}
}

func remoteFolders(remote *[]drive.File) *map[string]drive.File {
	folders := make(map[string]drive.File) // key: File.Id
	for _, file := range *remote {
		if file.MimeType == "application/vnd.google-apps.folder" {
			folders[file.Id] = file
		}
	}
	return &folders
}

func main() {
	basePath := os.Args[1]

	files := readFilesJson()
	srv := driveService()
	if len(files.Remote) == 0 {
		files.Remote = remote(srv)
	}
	if len(files.Local) == 0 {
		files.Local = local(basePath)
	}
	writeFilesJson(files)

	folders := remoteFolders(&files.Remote)


	localByMd5 := make(map[string]*localFile)
	for _, file := range files.Local {
		localByMd5[file.Md5Checksum] = &file
	}
	for _, remote := range files.Remote {
		if remote.Md5Checksum != "" {
			local := localByMd5[remote.Md5Checksum]
			if local == nil {
				path := remotePath(*folders, remote)
				fmt.Printf("%s (md5=%s)\n", path, remote.Md5Checksum)
				// download
				localPath := filepath.Join(basePath, path)
				fmt.Printf("=> %s\n", localPath)
				resp, err := srv.Files.Get(remote.Id).Download()
				if err != nil {
					log.Fatalf("Download failed: %v", err)
				}
				defer resp.Body.Close()
				out, err := os.Create(localPath)
				if err != nil {
					log.Fatalf("os.Create(%s) failed: %v", localPath, err)
				}
				defer out.Close()
				io.Copy(out, resp.Body)
				break
			}
			// break
		}
	}
	fmt.Printf("Those remote files above don't exist local.\n")
}
