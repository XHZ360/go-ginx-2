package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/testutil"
	"github.com/simp-frp/go-ginx-2/internal/adminapi"
)

func main() {
	outputPath := flag.String("output", "", "write the introspection schema JSON to this path")
	flag.Parse()

	schema, err := adminapi.GraphQLSchema()
	if err != nil {
		exitf("build admin GraphQL schema: %v", err)
	}

	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: testutil.IntrospectionQuery,
	})
	if len(result.Errors) > 0 {
		messages := make([]string, 0, len(result.Errors))
		for _, current := range result.Errors {
			messages = append(messages, current.Message)
		}
		exitf("introspect admin GraphQL schema: %s", strings.Join(messages, "; "))
	}

	payload, err := json.MarshalIndent(map[string]interface{}{"data": result.Data}, "", "  ")
	if err != nil {
		exitf("encode admin GraphQL schema: %v", err)
	}
	payload = append(payload, '\n')

	if *outputPath == "" {
		if _, err := os.Stdout.Write(payload); err != nil {
			exitf("write schema to stdout: %v", err)
		}
		return
	}

	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		exitf("create schema output directory: %v", err)
	}
	if err := os.WriteFile(*outputPath, payload, 0o644); err != nil {
		exitf("write schema to %s: %v", *outputPath, err)
	}
}

func exitf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
