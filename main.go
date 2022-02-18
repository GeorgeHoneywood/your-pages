package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type Website struct {
	ID         int       `db:"id"`
	Hostname   string    `db:"hostname"`
	UpdateTime time.Time `db:"update_time"`
}

type File struct {
	ID         int       `db:"id"`
	WebsiteID  int       `db:"website_id"`
	Path       string    `db:"path"`
	UpdateTime time.Time `db:"update_time"`
	Blob       []byte    `db:"blob"`
}

var TABLES string = `
CREATE TABLE IF NOT EXISTS website (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    hostname VARCHAR NOT NULL,
    update_time TIMESTAMP NOT NULL,
    UNIQUE(hostname)
);

CREATE TABLE IF NOT EXISTS file (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    website_id INTEGER NOT NULL,
    path VARCHAR NOT NULL,
    update_time TIMESTAMP NOT NULL,
    blob BLOB NOT NULL,
    UNIQUE(website_id, path)
    FOREIGN KEY(website_id) REFERENCES website(id)
);
`

var db *sqlx.DB

func main() {
	fmt.Println("serving your pages")
	var err error
	db, err = sqlx.Connect("sqlite", "your-pages.db")
	if err != nil {
		fmt.Printf("could not open database: %s\n", err)
	}
	defer db.Close()

	// setup database
	db.MustExec(TABLES)

	mux := http.NewServeMux()
	mux.HandleFunc("/upload", UploadHandler)
	mux.HandleFunc("/", ServeHandler)

	log.Fatal(http.ListenAndServe(":4444", mux))
}

func UploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "only HTTP posts requests accepted", http.StatusMethodNotAllowed)
		return
	}

	// // TODO: use a multipart reader
	// mpr, err := r.MultipartReader()
	// for {
	// 	part, err := mpr.NextPart()
	// 	part.FileName()
	// }

	r.ParseMultipartForm(32 << 20)
	sites := []string{}
	for site := range r.MultipartForm.File {
		sites = append(sites, site)
	}

	file, header, err := r.FormFile(sites[0])
	if err != nil {
		http.Error(w, fmt.Sprint("could not read file: ", err), http.StatusBadRequest)
	}

	tx := db.MustBegin()
	// calling rollback after commit is safe
	defer tx.Rollback()

	website := &Website{
		Hostname:   sites[0],
		UpdateTime: time.Now(),
	}

	res, err := tx.NamedQuery(`
        INSERT INTO website (hostname, update_time)
        VALUES (:hostname, :update_time)
        ON CONFLICT(hostname) DO UPDATE SET update_time = :update_time
        RETURNING id;
        `,
		website)

	if err != nil {
		http.Error(w, fmt.Sprint("could not insert website: ", err), http.StatusBadRequest)
		return
	}

	if res.Next() {
		res.Scan(&website.ID)
	}

	mimeType := mime.TypeByExtension(filepath.Ext(header.Filename))
	if mimeType != "application/gzip" {
		http.Error(w, "only gzip files accepted", http.StatusBadRequest)
		return
	}

	gzipped, err := gzip.NewReader(file)
	if err != nil {
		http.Error(w, fmt.Sprint("could not create GZIP reader: ", err), http.StatusInternalServerError)
	}
	gzipped.Close()

	tarred := tar.NewReader(gzipped)

	for {
		header, err := tarred.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, fmt.Sprint("could not read tar: ", err), http.StatusInternalServerError)
		}

		if header.Typeflag == tar.TypeDir {
			continue
		}

		// TODO: stream file into database
		bytes, err := io.ReadAll(tarred)
		if err != nil {
			http.Error(w, fmt.Sprint("could not read tar: ", err), http.StatusInternalServerError)
		}

		path := ""
		if filepath.Base(header.Name) == "index.html" {
			path = filepath.Dir(header.Name)
			if path == "." {
				path = ""
			} else {
				path += "/"
			}
		} else {
			path = header.Name
		}

		file := &File{
			WebsiteID:  website.ID,
			Path:       "/" + path,
			Blob:       bytes,
			UpdateTime: time.Now(),
		}

		_, err = tx.NamedExec(`
            INSERT INTO file (website_id, path, blob, update_time)
            VALUES (:website_id, :path, :blob, :update_time)
            ON CONFLICT(website_id, path) DO UPDATE SET blob = :blob, update_time = :update_time;
            `, file)
		if err != nil {
			http.Error(w, fmt.Sprint("could not insert file: ", err), http.StatusInternalServerError)
			return
		}
	}

	// TODO: clear out files that are no longer in the tar
	// could be done by checking if the update time if before/after the request started

	err = tx.Commit()
	if err != nil {
		http.Error(w, fmt.Sprint("could not commit transaction: ", err), http.StatusInternalServerError)
	}

	fmt.Fprintf(w, "uploaded site %s", sites[0])
}

func ServeHandler(w http.ResponseWriter, r *http.Request) {
	host := strings.Split(r.Host, ":")[0]

	website := &Website{}
	db.Get(website, "SELECT * FROM website WHERE hostname = $1;", host)
	if website.ID == 0 {
		http.Error(w, "website not found", http.StatusNotFound)
		return
	}

	file := &File{}
	// bit of magic to match paths with or without trailing slashes
	err := db.Get(file, `SELECT * FROM file WHERE website_id = $1 AND (path = $2 OR path = $2 || '/');`, website.ID, r.URL.Path)
	if err != nil {
		http.Error(w, fmt.Sprint("file not found: ", err), http.StatusNotFound)
	}

	http.ServeContent(w, r, file.Path, website.UpdateTime, bytes.NewReader(file.Blob))
}
