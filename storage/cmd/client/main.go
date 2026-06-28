package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"mrsydar/tagbase/storage/pkg/client"
)

func main() {
	slog.Debug("client starting")
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	baseURL, remaining := parseGlobalFlags(os.Args[1:])
	if baseURL == "" {
		fmt.Fprintf(os.Stderr, "error: --url is required\n")
		usage()
		os.Exit(1)
	}
	if len(remaining) < 1 {
		usage()
		os.Exit(1)
	}

	c := client.New(baseURL)
	ctx := context.Background()

	slog.Debug("executing command", "command", remaining[0])
	switch remaining[0] {
	case "list-collections":
		listCollections(ctx, c, remaining[1:])
	case "create-collection":
		createCollection(ctx, c, remaining[1:])
	case "upload":
		upload(ctx, c, remaining[1:])
	case "get":
		get(ctx, c, remaining[1:])
	case "data":
		getData(ctx, c, remaining[1:])
	case "tags":
		getTags(ctx, c, remaining[1:])
	case "query":
		query(ctx, c, remaining[1:])
	case "delete":
		deleteObject(ctx, c, remaining[1:])
	case "delete-collection":
		deleteCollection(ctx, c, remaining[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", remaining[0])
		usage()
		os.Exit(1)
	}
}

func usage() {
	slog.Debug("printing usage")
	fmt.Fprintf(os.Stderr, `usage: client --url <service-url> <command> [options]

commands:
  list-collections
  create-collection  --name <name> --data-type <type>
  delete-collection  --collection <c>
  upload             --collection <c> --data-type <type> --file <path> [--date <RFC3339>] [--ttl <seconds>]
  get                --collection <c> --id <id>
  data               --collection <c> --id <id> [--out <path>]
  tags               --collection <c> --id <id> [--tags <a,b>]
  query              --collection <c> [--tags <json>] [--limit <n>] [--cursor <cursor>] [--timeout <ms>]
  delete             --collection <c> --id <id>
`)
}

func parseGlobalFlags(args []string) (string, []string) {
	slog.Debug("parsing global flags")
	var url string
	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--url" {
			if i+1 < len(args) {
				url = args[i+1]
				i++
			}
			continue
		}
		if strings.HasPrefix(args[i], "--url=") {
			url = strings.TrimPrefix(args[i], "--url=")
			continue
		}
		out = append(out, args[i])
	}
	return url, out
}

func createCollection(ctx context.Context, c *client.Client, args []string) {
	slog.Debug("createCollection called")
	fs := flag.NewFlagSet("create-collection", flag.ExitOnError)
	name := fs.String("name", "", "collection name")
	dataType := fs.String("data-type", "", "data type")
	fs.Parse(args)
	if *name == "" || *dataType == "" {
		fs.Usage()
		os.Exit(1)
	}
	coll, err := c.CreateCollection(ctx, *name, *dataType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printJSON(coll)
}

func listCollections(ctx context.Context, c *client.Client, args []string) {
	slog.Debug("listCollections called")
	colls, err := c.ListCollections(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printJSON(colls)
}

func deleteCollection(ctx context.Context, c *client.Client, args []string) {
	slog.Debug("deleteCollection called")
	fs := flag.NewFlagSet("delete-collection", flag.ExitOnError)
	collection := fs.String("collection", "", "collection name")
	fs.Parse(args)
	if *collection == "" {
		fs.Usage()
		os.Exit(1)
	}
	if err := c.DeleteCollection(ctx, *collection); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("deleted")
}

func upload(ctx context.Context, c *client.Client, args []string) {
	slog.Debug("upload called")
	fs := flag.NewFlagSet("upload", flag.ExitOnError)
	collection := fs.String("collection", "", "collection name")
	dataType := fs.String("data-type", "", "data type")
	file := fs.String("file", "", "file to upload (use - for stdin)")
	dateStr := fs.String("date", "", "object date (RFC3339)")
	ttl := fs.Int("ttl", 0, "TTL in seconds")
	fs.Parse(args)
	if *collection == "" || *dataType == "" || *file == "" {
		fs.Usage()
		os.Exit(1)
	}
	var data []byte
	var err error
	if *file == "-" {
		data, err = readStdin()
	} else {
		data, err = os.ReadFile(*file)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
		os.Exit(1)
	}
	var t time.Time
	if *dateStr != "" {
		t, err = time.Parse(time.RFC3339, *dateStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing date: %v\n", err)
			os.Exit(1)
		}
	}
	resp, err := c.UploadObject(ctx, *collection, *dataType, data, t, *ttl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printJSON(resp)
}

func get(ctx context.Context, c *client.Client, args []string) {
	slog.Debug("get called")
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	collection := fs.String("collection", "", "collection name")
	id := fs.String("id", "", "object id")
	fs.Parse(args)
	if *collection == "" || *id == "" {
		fs.Usage()
		os.Exit(1)
	}
	obj, err := c.GetObjectMetadata(ctx, *collection, *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printJSON(obj)
}

func getData(ctx context.Context, c *client.Client, args []string) {
	slog.Debug("getData called")
	fs := flag.NewFlagSet("data", flag.ExitOnError)
	collection := fs.String("collection", "", "collection name")
	id := fs.String("id", "", "object id")
	out := fs.String("out", "-", "output file (default stdout)")
	fs.Parse(args)
	if *collection == "" || *id == "" {
		fs.Usage()
		os.Exit(1)
	}
	data, err := c.GetObjectData(ctx, *collection, *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if *out == "-" {
		os.Stdout.Write(data)
	} else {
		if err := os.WriteFile(*out, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(*out)
	}
}

func getTags(ctx context.Context, c *client.Client, args []string) {
	slog.Debug("getTags called")
	fs := flag.NewFlagSet("tags", flag.ExitOnError)
	collection := fs.String("collection", "", "collection name")
	id := fs.String("id", "", "object id")
	tags := fs.String("tags", "", "comma-separated tag names")
	fs.Parse(args)
	if *collection == "" || *id == "" {
		fs.Usage()
		os.Exit(1)
	}
	var tagList []string
	if *tags != "" {
		tagList = strings.Split(*tags, ",")
	}
	resp, err := c.GetObjectTags(ctx, *collection, *id, tagList)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printJSON(resp)
}

func query(ctx context.Context, c *client.Client, args []string) {
	slog.Debug("query called")
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	collection := fs.String("collection", "", "collection name")
	tagsJson := fs.String("tags", "", "JSON object of tag:true/false")
	limit := fs.Int("limit", 100, "result limit")
	cursor := fs.String("cursor", "", "pagination cursor")
	timeout := fs.Int("timeout", 30000, "query timeout in milliseconds")
	fs.Parse(args)
	if *collection == "" {
		fs.Usage()
		os.Exit(1)
	}
	req := client.TagsQueryRequest{Limit: *limit, Cursor: *cursor, TimeoutMs: *timeout}
	if *tagsJson != "" {
		if err := json.Unmarshal([]byte(*tagsJson), &req.Tags); err != nil {
			fmt.Fprintf(os.Stderr, "error parsing tags JSON: %v\n", err)
			os.Exit(1)
		}
	}
	resp, err := c.QueryObjects(ctx, *collection, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printJSON(resp)
}

func deleteObject(ctx context.Context, c *client.Client, args []string) {
	slog.Debug("deleteObject called")
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	collection := fs.String("collection", "", "collection name")
	id := fs.String("id", "", "object id")
	fs.Parse(args)
	if *collection == "" || *id == "" {
		fs.Usage()
		os.Exit(1)
	}
	if err := c.DeleteObject(ctx, *collection, *id); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("deleted")
}

func printJSON(v any) {
	slog.Debug("printJSON called")
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func readStdin() ([]byte, error) {
	slog.Debug("readStdin called")
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			b.Write(buf[:n])
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
	}
	return []byte(b.String()), nil
}
