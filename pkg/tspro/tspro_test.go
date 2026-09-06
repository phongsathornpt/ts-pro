package tspro

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompilerEndToEnd(t *testing.T) {
	src := []byte(`
function add(a: number, b: number): number {
    return a + b;
}
`)

	c := New(Options{
		TargetOS:   "darwin",
		TargetArch: "arm64",
		OptLevel:   2,
	})

	bin, diags, err := c.CompileSource("add.ts", src)
	if err != nil {
		t.Fatalf("CompileSource failed: %v (diags: %s)", err, diags.Format(c.FileSet()))
	}

	if len(bin) < 64 {
		t.Fatalf("generated binary too small: %d bytes", len(bin))
	}

	// Test Linux ELF target
	cLinux := New(Options{
		TargetOS:   "linux",
		TargetArch: "amd64",
		OptLevel:   2,
	})
	binLinux, _, err := cLinux.CompileSource("add.ts", src)
	if err != nil {
		t.Fatalf("Linux CompileSource failed: %v", err)
	}
	if len(binLinux) < 64 {
		t.Fatalf("Linux binary too small: %d bytes", len(binLinux))
	}
}

func TestCompilerCheck(t *testing.T) {
	src := []byte(`
let x: number = "mismatch";
`)
	c := New(DefaultOptions())
	diags := c.Check("test.ts", src)
	if !diags.HasErrors() {
		t.Errorf("expected type error, got none")
	}

	// Check syntax error
	badSyntax := []byte(`let x: = 123;`)
	diagsSyntax := c.Check("bad.ts", badSyntax)
	if !diagsSyntax.HasErrors() {
		t.Errorf("expected parse error, got none")
	}
}

func TestCompilerOptionsDefaults(t *testing.T) {
	cEmpty := New(Options{})
	if cEmpty.opts.TargetOS == "" || cEmpty.opts.TargetArch == "" {
		t.Errorf("expected defaults filled, got OS=%q Arch=%q", cEmpty.opts.TargetOS, cEmpty.opts.TargetArch)
	}
}

func TestCompileSourceRejections(t *testing.T) {
	c := New(DefaultOptions())

	// Syntax error
	_, _, err := c.CompileSource("bad.ts", []byte(`let x: = ;`))
	if err == nil {
		t.Errorf("expected error on bad syntax")
	}

	// Relative import rejected in CompileSource
	_, _, err = c.CompileSource("imp.ts", []byte(`import { x } from "./other";`))
	if err == nil || !strings.Contains(err.Error(), "relative imports require CompileFile") {
		t.Errorf("expected relative imports error, got %v", err)
	}

	// Unsupported OS target
	cBadOS := New(Options{TargetOS: "solaris", TargetArch: "amd64"})
	_, _, err = cBadOS.CompileSource("test.ts", []byte(`let x: number = 1;`))
	if err == nil {
		t.Errorf("expected error for unsupported target OS")
	}
}

func TestCompileFileModuleLoading(t *testing.T) {
	dir := t.TempDir()

	// Non-relative import rejection
	badImport := filepath.Join(dir, "bad_import.ts")
	if err := os.WriteFile(badImport, []byte(`import { x } from "pkg";`), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{TargetOS: "linux", TargetArch: "amd64"})
	_, err := c.CompileFile(badImport, filepath.Join(dir, "out1"))
	if err == nil || !strings.Contains(err.Error(), "only relative TypeScript imports are supported") {
		t.Errorf("expected non-relative import error, got %v", err)
	}

	// Missing module file
	missingImport := filepath.Join(dir, "missing_import.ts")
	if err := os.WriteFile(missingImport, []byte(`import { x } from "./nonexistent";`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = c.CompileFile(missingImport, filepath.Join(dir, "out2"))
	if err == nil || !strings.Contains(err.Error(), "cannot resolve module") {
		t.Errorf("expected cannot resolve module error, got %v", err)
	}

	// Valid relative import with directory index.ts
	modDir := filepath.Join(dir, "submod")
	if err := os.Mkdir(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "index.ts"), []byte(`export function helper(): number { return 42; }`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainTs := filepath.Join(dir, "main.ts")
	if err := os.WriteFile(mainTs, []byte(`import { helper } from "./submod"; console.log(helper());`), 0o644); err != nil {
		t.Fatal(err)
	}
	outBin := filepath.Join(dir, "out_main")
	diags, err := c.CompileFile(mainTs, outBin)
	if err != nil {
		t.Fatalf("CompileFile failed: %v, diags: %v", err, diags)
	}
	if info, err := os.Stat(outBin); err != nil || info.Size() == 0 {
		t.Errorf("expected output binary created, info: %v, err: %v", info, err)
	}
}

func TestCompileFileRejectsCyclicRelativeImports(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.ts")
	b := filepath.Join(dir, "b.ts")
	if err := os.WriteFile(a, []byte(`import { b } from "./b"; export function a(): number { return b(); }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`import { a } from "./a"; export function b(): number { return a(); }`), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	_, err := c.CompileFile(a, filepath.Join(dir, "out"))
	if err == nil || !strings.Contains(err.Error(), "cyclic module import") {
		t.Fatalf("CompileFile cycle error = %v", err)
	}
}

func TestTsproDirect100Cover(t *testing.T) {
	c := New(Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	_ = c.FileSet()

	dir := t.TempDir()
	srcFile := filepath.Join(dir, "ok.ts")
	_ = os.WriteFile(srcFile, []byte("console.log(1);"), 0o644)

	// compileProgram type checking error
	_, _, err := c.CompileSource("bad_type.ts", []byte("let x: number = \"str\";"))
	if err == nil {
		t.Error("expected sema error in compileProgram")
	}

	// loadModule syntax error in imported module
	badSyntaxMod := filepath.Join(dir, "bad_syntax.ts")
	_ = os.WriteFile(badSyntaxMod, []byte("export const x = ;"), 0o644)
	mainImportBad := filepath.Join(dir, "main_bad.ts")
	_ = os.WriteFile(mainImportBad, []byte("import { x } from \"./bad_syntax\";"), 0o644)
	_, err = c.CompileFile(mainImportBad, filepath.Join(dir, "out4"))
	if err == nil {
		t.Error("expected syntax error in imported module")
	}

	// CompileFile compilation error (e.g. invalid target OS)
	cBadTgt := New(Options{TargetOS: "invalid_os", TargetArch: "invalid_arch"})
	_, err = cBadTgt.CompileFile(srcFile, filepath.Join(dir, "out5"))
	if err == nil {
		t.Error("expected error for bad target in CompileFile")
	}

	// CompileFile error when input path fails
	_, err = c.CompileFile("/nonexistent_path/foo.ts", filepath.Join(dir, "out"))
	if err == nil {
		t.Error("expected error for nonexistent input file")
	}

	// CompileFile error when output path is unwritable directory
	unwritableDir := filepath.Join(dir, "unwritable_dir")
	_ = os.Mkdir(unwritableDir, 0o555)
	_, err = c.CompileFile(srcFile, filepath.Join(unwritableDir, "sub", "out"))
	if err == nil {
		t.Error("expected error for unwritable output")
	}

	// CompileProgram error when target is invalid
	cInvalid := New(Options{TargetOS: "bad_os", TargetArch: "bad_arch"})
	_, _, err = cInvalid.CompileSource("test.ts", []byte("let x = 1;"))
	if err == nil {
		t.Error("expected error for bad target")
	}

	// loadModule diamond import (module imported twice via different paths)
	sharedMod := filepath.Join(dir, "shared.ts")
	_ = os.WriteFile(sharedMod, []byte("export const val = 100;"), 0o644)
	midModA := filepath.Join(dir, "mid_a.ts")
	_ = os.WriteFile(midModA, []byte("import { val } from \"./shared\"; export const a = val;"), 0o644)
	midModB := filepath.Join(dir, "mid_b.ts")
	_ = os.WriteFile(midModB, []byte("import { val } from \"./shared\"; export const b = val;"), 0o644)
	diamondMain := filepath.Join(dir, "diamond.ts")
	_ = os.WriteFile(diamondMain, []byte("import { a } from \"./mid_a\"; import { b } from \"./mid_b\"; console.log(a + b);"), 0o644)
	_, err = c.CompileFile(diamondMain, filepath.Join(dir, "out_diamond"))
	if err != nil {
		t.Logf("diamond diags: %v", err)
	}

	// loadModule non-relative import error
	badImportTs := filepath.Join(dir, "bad_import.ts")
	_ = os.WriteFile(badImportTs, []byte("import { a } from \"pkg\";"), 0o644)
	_, err = c.CompileFile(badImportTs, filepath.Join(dir, "out3"))
	if err == nil {
		t.Error("expected error for non-relative import")
	}
}

func TestCompilerReleasesPreviousCompilationFileSet(t *testing.T) {
	c := New(Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	src := []byte(`function add(a: number, b: number): number { return a + b; }`)

	var previous = c.FileSet()
	for i := 0; i < 64; i++ {
		if _, diags, err := c.CompileSource("repeat.ts", src); err != nil {
			t.Fatalf("compile %d: %v (%s)", i, err, diags.Format(c.FileSet()))
		}
		current := c.FileSet()
		if current == previous {
			t.Fatalf("compile %d reused FileSet; previous sources would remain retained", i)
		}
		previous = current
	}
}
