package codeindex

import (
	"strings"
	"testing"
)

func TestExtractGoItems(t *testing.T) {
	result := Extract("cmd/app/main.go", "go", []byte(`package main

import "fmt"

type Server struct{}
type hidden struct{}

func main() {}
func Start() {}
func helper() {}

var _ = fmt.Println
`))
	assertItem(t, result.Items, "go_package", "main")
	assertItem(t, result.Items, "go_import", "fmt")
	assertItem(t, result.Items, "go_func", "Start")
	assertItem(t, result.Items, "go_type", "Server")
	assertItem(t, result.Items, "go_main", "main")
	assertNoItem(t, result.Items, "go_func", "helper")
	assertNoItem(t, result.Items, "go_type", "hidden")
}

func TestExtractPythonItems(t *testing.T) {
	result := Extract("pkg/tools.py", "python", []byte(`import os
from sys import path

def build():
    pass

def _helper():
    pass

class Runner:
    def start(self):
        pass
    def _stop(self):
        pass

class _Hidden:
    pass
`))
	assertItem(t, result.Items, "py_import", "import os")
	assertItem(t, result.Items, "py_import", "from sys import path")
	assertItem(t, result.Items, "py_func", "build")
	assertItem(t, result.Items, "py_class", "Runner")
	assertItem(t, result.Items, "py_method", "start")
	assertNoItem(t, result.Items, "py_func", "_helper")
	assertNoItem(t, result.Items, "py_class", "_Hidden")
	assertNoItem(t, result.Items, "py_method", "_stop")
}

func TestExtractJavaScriptItems(t *testing.T) {
	result := Extract("ui/app.js", "javascript", []byte(`import thing from "pkg";

export function run() {}
export class Widget {
  render() {}
}
function local() {}
`))
	assertItem(t, result.Items, "js_import", "pkg")
	assertItem(t, result.Items, "js_func", "run")
	assertItem(t, result.Items, "js_class", "Widget")
	assertItem(t, result.Items, "js_method", "render")
	assertNoItem(t, result.Items, "js_func", "local")
}

func TestExtractJSXItems(t *testing.T) {
	result := Extract("ui/app.jsx", "jsx", []byte(`import React from "react";

export function View(props) {
  return <div>{props.title}</div>;
}
function local() {}
`))
	assertItem(t, result.Items, "js_import", "react")
	assertItem(t, result.Items, "js_func", "View")
	assertNoItem(t, result.Items, "js_func", "local")
}

func TestExtractTypeScriptItems(t *testing.T) {
	result := Extract("ui/app.ts", "typescript", []byte(`import { thing } from "pkg";

export function run(): void {}
export interface Shape { size: number }
export type Alias = string;
export class Widget {
  render(): void {}
}
function local() {}
`))
	assertItem(t, result.Items, "ts_import", "pkg")
	assertItem(t, result.Items, "ts_func", "run")
	assertItem(t, result.Items, "ts_interface", "Shape")
	assertItem(t, result.Items, "ts_type", "Alias")
	assertItem(t, result.Items, "ts_class", "Widget")
	assertItem(t, result.Items, "ts_method", "render")
	assertNoItem(t, result.Items, "ts_func", "local")
}

func TestExtractTSXItems(t *testing.T) {
	result := Extract("ui/app.tsx", "tsx", []byte(`import React from "react";

export interface Props { title: string }
export function View(props: Props) {
  return <div>{props.title}</div>;
}
const local = () => null;
`))
	assertItem(t, result.Items, "ts_import", "react")
	assertItem(t, result.Items, "ts_interface", "Props")
	assertItem(t, result.Items, "ts_func", "View")
	assertNoItem(t, result.Items, "ts_func", "local")
}

func TestExtractCSharpItems(t *testing.T) {
	result := Extract("src/App.cs", "csharp", []byte(`using System;

namespace Demo.App;

public interface IService {}

public class Runner {
  public void Start() {}
  private void Stop() {}
}
`))
	assertItem(t, result.Items, "cs_using", "System")
	assertItem(t, result.Items, "cs_namespace", "Demo.App")
	assertItem(t, result.Items, "cs_interface", "IService")
	assertItem(t, result.Items, "cs_class", "Runner")
	assertItem(t, result.Items, "cs_method", "Start")
	assertNoItem(t, result.Items, "cs_method", "Stop")
}

func TestExtractParseFailureWarns(t *testing.T) {
	result := Extract("bad.py", "python", []byte("def {\n"))
	if len(result.Items) != 0 {
		t.Fatalf("items = %#v, want none", result.Items)
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "syntax errors") {
		t.Fatalf("warnings = %#v, want syntax warning", result.Warnings)
	}
}

func TestExtractCommonJSRequire(t *testing.T) {
	result := Extract("lib/app.js", "javascript", []byte(`const express = require("express")
var path = require("path")
let debug = require("debug")("app")
`))
	for _, name := range []string{"express", "path", "debug"} {
		assertItem(t, result.Items, "js_import", name)
	}
	// require bindings are import-like, not definitions.
	for _, item := range result.Items {
		if item.Kind == "js_import" && !item.IsImportLike() {
			t.Fatalf("js_import should be import-like: %#v", item)
		}
	}
}

func TestExtractCommonJSModuleExportsIdentifier(t *testing.T) {
	result := Extract("lib/app.js", "javascript", []byte(`function createApplication() {}
module.exports = createApplication
`))
	assertItem(t, result.Items, "js_export", "createApplication")
	// The export hint is a definition, not import-like.
	for _, item := range result.Items {
		if item.Kind == "js_export" && item.IsImportLike() {
			t.Fatalf("js_export should not be import-like: %#v", item)
		}
	}
}

func TestExtractCommonJSExportsMember(t *testing.T) {
	result := Extract("lib/router.js", "javascript", []byte(`function Router() {}
exports.Router = Router
module.exports.createApplication = createApplication
`))
	assertItem(t, result.Items, "js_export", "Router")
	assertItem(t, result.Items, "js_export", "createApplication")
}

func TestExtractCommonJSChainedExports(t *testing.T) {
	// Express's actual pattern: `exports = module.exports = createApplication`.
	result := Extract("lib/express.js", "javascript", []byte(`function createApplication() {}
exports = module.exports = createApplication
`))
	assertItem(t, result.Items, "js_export", "createApplication")
}

func TestExtractCommonJSObjectExports(t *testing.T) {
	result := Extract("lib/index.js", "javascript", []byte(`module.exports = {
  createApplication,
  Router,
}
`))
	assertItem(t, result.Items, "js_export", "createApplication")
	assertItem(t, result.Items, "js_export", "Router")
}

func TestExtractCommonJSDoesNotOverExtract(t *testing.T) {
	// Plain top-level statements that are not require/module.exports must not
	// produce code items.
	result := Extract("lib/run.js", "javascript", []byte(`const port = 3000
app.listen(port)
config.value = 1
`))
	if len(result.Items) != 0 {
		t.Fatalf("expected no items for non-CommonJS statements, got %#v", result.Items)
	}
}

func assertItem(t *testing.T, items []Item, kind string, name string) {
	t.Helper()
	for _, item := range items {
		if item.Kind == kind && item.Name == name {
			return
		}
	}
	t.Fatalf("missing item kind=%s name=%s in %#v", kind, name, items)
}

func assertNoItem(t *testing.T, items []Item, kind string, name string) {
	t.Helper()
	for _, item := range items {
		if item.Kind == kind && item.Name == name {
			t.Fatalf("unexpected item kind=%s name=%s in %#v", kind, name, items)
		}
	}
}
