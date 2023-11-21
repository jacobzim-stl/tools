// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"context"
	"fmt"
	"go/ast"
	"go/scanner"
	"go/token"
	"go/types"
	"sync"

	"golang.org/x/tools/gopls/internal/lsp/cache/metadata"
	"golang.org/x/tools/gopls/internal/lsp/cache/methodsets"
	"golang.org/x/tools/gopls/internal/lsp/cache/parsego"
	"golang.org/x/tools/gopls/internal/lsp/cache/xrefs"
	"golang.org/x/tools/gopls/internal/lsp/protocol"
	"golang.org/x/tools/gopls/internal/lsp/source"
)

// Temporary refactoring, reversing the source import:
// Types
type (
	// Metadata.
	PackageID   = metadata.PackageID
	PackagePath = metadata.PackagePath
	PackageName = metadata.PackageName
	ImportPath  = metadata.ImportPath
	Metadata    = metadata.Metadata

	// Diagnostics.
	Diagnostic   = source.Diagnostic
	SuggestedFix = source.SuggestedFix

	// Computed objects.
	TidiedModule   = source.TidiedModule
	ParsedGoFile   = source.ParsedGoFile
	ParsedModule   = source.ParsedModule
	ParsedWorkFile = source.ParsedWorkFile
	Package_       = source.Package // renamed to avoid conflict
	Symbol         = source.Symbol

	XrefIndex_       = source.XrefIndex // renamed to avoid conflict
	GlobalSnapshotID = source.GlobalSnapshotID
	InvocationFlags  = source.InvocationFlags
	CriticalError    = source.CriticalError
	Filterer         = source.Filterer
)

// Values
var (
	// Parse Modes
	ParseHeader = parsego.ParseHeader
	ParseFull   = parsego.ParseFull

	// Diagnostic sources.
	ModTidyError = source.ModTidyError
	ListError    = source.ListError
	ParseError   = source.ParseError
	TypeError    = source.TypeError

	// Invocation flags.
	Normal                = source.Normal
	AllowNetwork          = source.AllowNetwork
	LoadWorkspace         = source.LoadWorkspace
	WriteTemporaryModFile = source.WriteTemporaryModFile
)

// Functions
var (
	IsValidImport                  = source.IsValidImport
	RemoveIntermediateTestVariants = source.RemoveIntermediateTestVariants
	NewFilterer                    = source.NewFilterer
	BuildLink                      = source.BuildLink
	CanFix                         = source.CanFix
	AnalyzerErrorKind              = source.AnalyzerErrorKind
)

// A Package is the union of package metadata and type checking results.
//
// TODO(rfindley): for now, we do not persist the post-processing of
// loadDiagnostics, because the value of the snapshot.packages map is just the
// package handle. Fix this.
type Package struct {
	m               *Metadata
	loadDiagnostics []*Diagnostic
	pkg             *syntaxPackage
}

// syntaxPackage contains parse trees and type information for a package.
type syntaxPackage struct {
	// -- identifiers --
	id PackageID

	// -- outputs --
	fset            *token.FileSet // for now, same as the snapshot's FileSet
	goFiles         []*ParsedGoFile
	compiledGoFiles []*ParsedGoFile
	diagnostics     []*Diagnostic
	parseErrors     []scanner.ErrorList
	typeErrors      []types.Error
	types           *types.Package
	typesInfo       *types.Info
	importMap       map[PackagePath]*types.Package

	xrefsOnce sync.Once
	_xrefs    []byte // only used by the xrefs method

	methodsetsOnce sync.Once
	_methodsets    *methodsets.Index // only used by the methodsets method
}

func (p *syntaxPackage) xrefs() []byte {
	p.xrefsOnce.Do(func() {
		p._xrefs = xrefs.Index(p.compiledGoFiles, p.types, p.typesInfo)
	})
	return p._xrefs
}

func (p *syntaxPackage) methodsets() *methodsets.Index {
	p.methodsetsOnce.Do(func() {
		p._methodsets = methodsets.NewIndex(p.fset, p.types)
	})
	return p._methodsets
}

func (p *Package) String() string { return string(p.m.ID) }

func (p *Package) Metadata() *Metadata { return p.m }

// A loadScope defines a package loading scope for use with go/packages.
//
// TODO(rfindley): move this to load.go.
type loadScope interface {
	aScope()
}

type (
	fileLoadScope    protocol.DocumentURI // load packages containing a file (including command-line-arguments)
	packageLoadScope string               // load a specific package (the value is its PackageID)
	moduleLoadScope  struct {
		dir        string // dir containing the go.mod file
		modulePath string // parsed module path
	}
	viewLoadScope protocol.DocumentURI // load the workspace
)

// Implement the loadScope interface.
func (fileLoadScope) aScope()    {}
func (packageLoadScope) aScope() {}
func (moduleLoadScope) aScope()  {}
func (viewLoadScope) aScope()    {}

func (p *Package) CompiledGoFiles() []*ParsedGoFile {
	return p.pkg.compiledGoFiles
}

func (p *Package) File(uri protocol.DocumentURI) (*ParsedGoFile, error) {
	return p.pkg.File(uri)
}

func (pkg *syntaxPackage) File(uri protocol.DocumentURI) (*ParsedGoFile, error) {
	for _, cgf := range pkg.compiledGoFiles {
		if cgf.URI == uri {
			return cgf, nil
		}
	}
	for _, gf := range pkg.goFiles {
		if gf.URI == uri {
			return gf, nil
		}
	}
	return nil, fmt.Errorf("no parsed file for %s in %v", uri, pkg.id)
}

func (p *Package) GetSyntax() []*ast.File {
	var syntax []*ast.File
	for _, pgf := range p.pkg.compiledGoFiles {
		syntax = append(syntax, pgf.File)
	}
	return syntax
}

func (p *Package) FileSet() *token.FileSet {
	return p.pkg.fset
}

func (p *Package) GetTypes() *types.Package {
	return p.pkg.types
}

func (p *Package) GetTypesInfo() *types.Info {
	return p.pkg.typesInfo
}

// DependencyTypes returns the type checker's symbol for the specified
// package. It returns nil if path is not among the transitive
// dependencies of p, or if no symbols from that package were
// referenced during the type-checking of p.
func (p *Package) DependencyTypes(path PackagePath) *types.Package {
	return p.pkg.importMap[path]
}

func (p *Package) GetParseErrors() []scanner.ErrorList {
	return p.pkg.parseErrors
}

func (p *Package) GetTypeErrors() []types.Error {
	return p.pkg.typeErrors
}

func (p *Package) DiagnosticsForFile(ctx context.Context, uri protocol.DocumentURI) ([]*Diagnostic, error) {
	var diags []*Diagnostic
	for _, diag := range p.loadDiagnostics {
		if diag.URI == uri {
			diags = append(diags, diag)
		}
	}
	for _, diag := range p.pkg.diagnostics {
		if diag.URI == uri {
			diags = append(diags, diag)
		}
	}

	return diags, nil
}
