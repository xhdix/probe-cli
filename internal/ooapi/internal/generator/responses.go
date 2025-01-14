package main

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

func (d *Descriptor) genNewResponse(sb *strings.Builder) {
	fmt.Fprintf(sb,
		"func (api *%s) newResponse(ctx context.Context, resp *http.Response, err error) (%s, error) {\n",
		d.APIStructName(), d.ResponseTypeName())

	fmt.Fprint(sb, "\tif err != nil {\n")
	fmt.Fprint(sb, "\t\treturn nil, err\n")
	fmt.Fprint(sb, "\t}\n")
	fmt.Fprint(sb, "\tif resp.StatusCode == 401 {\n")
	fmt.Fprint(sb, "\t\treturn nil, ErrUnauthorized\n")
	fmt.Fprint(sb, "\t}\n")
	fmt.Fprint(sb, "\tif resp.StatusCode != 200 {\n")
	fmt.Fprint(sb, "\t\treturn nil, newHTTPFailure(resp.StatusCode)\n")
	fmt.Fprint(sb, "\t}\n")
	fmt.Fprint(sb, "\tdefer resp.Body.Close()\n")
	fmt.Fprint(sb, "\treader := io.LimitReader(resp.Body, 4<<20)\n")
	fmt.Fprint(sb, "\tdata, err := netxlite.ReadAllContext(ctx, reader)\n")
	fmt.Fprint(sb, "\tif err != nil {\n")
	fmt.Fprint(sb, "\t\treturn nil, err\n")
	fmt.Fprint(sb, "\t}\n")

	switch d.ResponseTypeKind() {
	case reflect.Map:
		fmt.Fprintf(sb, "\tout := %s{}\n", d.ResponseTypeName())
	case reflect.Struct:
		fmt.Fprintf(sb, "\tout := &%s{}\n", d.ResponseTypeNameAsStruct())
	}

	switch d.ResponseTypeKind() {
	case reflect.Map:
		fmt.Fprint(sb, "\tif err := api.jsonCodec().Decode(data, &out); err != nil {\n")
	case reflect.Struct:
		fmt.Fprint(sb, "\tif err := api.jsonCodec().Decode(data, out); err != nil {\n")
	}

	fmt.Fprint(sb, "\t\treturn nil, err\n")
	fmt.Fprint(sb, "\t}\n")

	switch d.ResponseTypeKind() {
	case reflect.Map:
		// For rationale, see https://play.golang.org/p/m9-MsTaQ5wt and
		// https://play.golang.org/p/6h-v-PShMk9.
		fmt.Fprint(sb, "\tif out == nil {\n")
		fmt.Fprint(sb, "\t\treturn nil, ErrJSONLiteralNull\n")
		fmt.Fprint(sb, "\t}\n")
	case reflect.Struct:
		// nothing
	}
	fmt.Fprintf(sb, "\treturn out, nil\n")
	fmt.Fprintf(sb, "}\n\n")
}

// GenResponsesGo generates responses.go.
func GenResponsesGo(file string) {
	var sb strings.Builder
	fmt.Fprint(&sb, "// Code generated by go generate; DO NOT EDIT.\n")
	fmt.Fprintf(&sb, "// %s\n\n", time.Now())
	fmt.Fprint(&sb, "package ooapi\n\n")
	fmt.Fprintf(&sb, "//go:generate go run ./internal/generator -file %s\n\n", file)
	fmt.Fprint(&sb, "import (\n")
	fmt.Fprint(&sb, "\t\"context\"\n")
	fmt.Fprint(&sb, "\t\"io\"\n")
	fmt.Fprint(&sb, "\t\"net/http\"\n")
	fmt.Fprint(&sb, "\n")
	fmt.Fprint(&sb, "\t\"github.com/ooni/probe-cli/v3/internal/netxlite\"\n")
	fmt.Fprint(&sb, "\t\"github.com/ooni/probe-cli/v3/internal/ooapi/apimodel\"\n")
	fmt.Fprint(&sb, ")\n\n")
	for _, desc := range Descriptors {
		desc.genNewResponse(&sb)
	}
	writefile(file, &sb)
}
