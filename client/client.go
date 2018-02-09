package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"text/tabwriter"

	cli "gopkg.in/urfave/cli.v1"
)

var (
	endpoint  string
	authToken string
	insecure  bool
)

func Request(method string, path string, request map[string]string) ([]byte, error) {
	uri, _ := url.Parse(endpoint)
	rel, _ := url.Parse(path)
	uri = uri.ResolveReference(rel)
	var req *http.Request
	var err error
	if len(request) > 0 {
		payload, _ := json.Marshal(request)
		req, err = http.NewRequest(method, uri.String(), bytes.NewReader(payload))
	} else {
		req, err = http.NewRequest(method, uri.String(), nil)
	}
	if err != nil {
		return nil, fmt.Errorf("creating login request: %v", err)
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+authToken)

	tr := &http.Transport{}
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{Transport: tr}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending HTTP request: %v", err)
	}
	defer res.Body.Close()

	body, _ := ioutil.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return body, errors.New(http.StatusText(res.StatusCode))
	}
	return body, nil
}

func POSTJSON(path string, request map[string]string) (map[string]string, error) {
	body, err := Request("POST", path, request)
	if err != nil {
		return nil, err
	}
	var res map[string]string
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&res); err != nil {
		return nil, err
	}
	return res, nil
}

func cmdLogin(c *cli.Context) error {
	// Perform user authentication to get a token.
	if c.NArg() != 2 {
		fmt.Println("Specify <username> <password>")
		return nil
	}
	res, err := POSTJSON("login", map[string]string{
		"email":    c.Args()[0],
		"password": c.Args()[1],
	})
	if err != nil {
		log.Fatal(err)
	}
	if res["token"] == "" {
		log.Fatal(res)
	}
	fmt.Println(res["token"])
	return nil
}

func cmdList(c *cli.Context) error {
	body, err := Request("GET", "/document/", nil)
	if err != nil {
		return fmt.Errorf("error in list request: %v", err)
	}
	var res []struct {
		Id, Userid, Name, MimeType string
		Size                       int
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&res); err != nil {
		return err
	}
	if len(res) == 0 {
		fmt.Println("No documents found")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "ID\tSize\tName")
	fmt.Fprintln(w, "------------------------------------\t------\t---------------\t")

	for _, row := range res {
		fmt.Fprintf(w, "%s\t%-d\t%s\t\n", row.Id, row.Size, row.Name)
	}
	w.Flush()
	return nil
}

func cmdUpload(c *cli.Context) error {
	if c.NArg() < 1 {
		cli.ShowCommandHelpAndExit(c, "upload", 1)
		return nil
	}
	body, err := ioutil.ReadFile(c.Args()[0])
	if err != nil {
		return fmt.Errorf("Error reading file: %v", err)
	}

	req := map[string]string{
		"body":      base64.StdEncoding.EncodeToString(body),
		"name":      c.Args()[c.NArg()-1],
		"mime_type": c.String("mime_type"),
	}
	res, err := Request("POST", "/document/", req)
	if err != nil {
		return fmt.Errorf("Error from server: %v", err)
	}
	var r map[string]interface{}
	if err := json.Unmarshal(res, &r); err != nil {
		return fmt.Errorf("Error decoding server response: %v", err)
	}
	fmt.Printf("Uploaded %q as document %q\n", r["name"], r["id"])
	return nil
}

func cmdDownload(c *cli.Context) error {
	if c.NArg() != 1 {
		cli.ShowCommandHelpAndExit(c, "download", 1)
		return nil
	}
	body, err := Request("GET", path.Join("/document", c.Args()[0]), nil)
	if err != nil {
		return fmt.Errorf("Error retrieving %s: %v\n", c.Args()[0], err)
	}
	os.Stdout.Write(body)
	return nil
}

func cmdDelete(c *cli.Context) error {
	if c.NArg() < 1 {
		cli.ShowCommandHelpAndExit(c, "delete", 1)
		return nil
	}
	for _, id := range c.Args() {
		_, err := Request("DELETE", path.Join("/document", id), nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting %s: %v\n", id, err)
			continue
		}
		fmt.Printf("Deleted document %s\n", id)
	}
	return nil
}

func main() {
	defaultEndpoint := "http://localhost/"
	if os.Getenv("PROJECT") != "" {
		defaultEndpoint = fmt.Sprintf("https://frontend.endpoints.%s.cloud.goog/", os.Getenv("PROJECT"))
		insecure = true
	}
	app := cli.NewApp()
	app.Name = "client"
	app.Usage = "Test Client for Web Application Service Demo"
	app.Version = "1.0.0"
	app.Flags = []cli.Flag{
		cli.StringFlag{Name: "token, t", Usage: "Authentication token", Destination: &authToken, EnvVar: "TOKEN"},
		cli.StringFlag{Name: "url, u", Usage: "Base URL of service", Value: defaultEndpoint, Destination: &endpoint},
		cli.BoolFlag{Name: "insecure", Usage: "Ignore invalid SSL certificates", Destination: &insecure},
	}
	app.Commands = []cli.Command{
		{
			Name:      "login",
			Usage:     "Login and generate a login token",
			Action:    cmdLogin,
			ArgsUsage: "<email> <password>",
		},
		{
			Name:   "list",
			Usage:  "List uploaded files",
			Action: cmdList,
		},
		{
			Name:      "upload",
			Aliases:   []string{"up", "put"},
			Usage:     "Upload a local file",
			Action:    cmdUpload,
			ArgsUsage: "<filename> [name]",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "mime_type", Usage: "MIME type of the file (default is autodetected)"},
			},
		},
		{
			Name:      "download",
			Aliases:   []string{"down", "get"},
			Usage:     "Download a document",
			Action:    cmdDownload,
			ArgsUsage: "<id>",
		},
		{
			Name:      "delete",
			Aliases:   []string{"del"},
			Usage:     "Delete a document",
			Action:    cmdDelete,
			ArgsUsage: "<id>",
		},
	}
	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
