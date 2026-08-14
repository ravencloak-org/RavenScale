// Package tenantcheck is a go/analysis pass that guards the tenant boundary
// (CONTEXT decision 2 / ADR-0007).
//
// The runtime auto-scoping callback in hscontrol/db/tenant_scope.go injects
// `WHERE tenant_id = ?` on every model-driven statement and fails closed. Its
// documented KNOWN GAP is that raw SQL and hand-rolled joins bypass the model
// callbacks entirely — the callback cannot close this, so this analyzer does.
//
// It flags, on *gorm.DB call chains, against tenant-scoped tables (users,
// nodes, pre_auth_keys, policies, tailnets):
//
//   - .Raw(...) / .Exec(...) whose SQL string references a tenant-scoped table
//     (bypasses the statement builder, so no callback runs), and
//   - .Joins(...) referencing a tenant-scoped table (only the primary model is
//     auto-scoped; a joined tenant table is read unscoped).
//
// This is a pragmatic bypass detector, not a theorem prover: it does not try to
// prove that an arbitrary query is correctly scoped. It only catches the two
// escape hatches the runtime gate provably cannot see.
//
// Escape hatch: a `//tenantcheck:allow <reason>` comment on the flagged line, or
// on the line directly above it, suppresses the finding. Whole files that are
// legitimately raw (the gormigrate migrations in hscontrol/db/db.go and
// hscontrol/db/versioncheck.go) are skipped by path, and `_test.go` files are
// skipped wholesale (test scaffolding deliberately bypasses scoping).
package tenantcheck

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// tenantTableRe matches any tenant-scoped table name as a whole word. These are
// the tables whose models carry a TenantID field (see tenant_scope.go). The
// non-scoped tables (api_keys, migrations, tenants) are deliberately absent.
var tenantTableRe = regexp.MustCompile(`\b(users|nodes|pre_auth_keys|policies|tailnets)\b`)

// defaultSkipSuffixes lists file-path suffixes that are exempt wholesale. These
// files are legitimately raw (schema migrations / version bookkeeping) and
// predate the tenant model; per-line directives would be noise. Overridable via
// the -skip flag.
var defaultSkipSuffixes = []string{
	"/hscontrol/db/db.go",
	"/hscontrol/db/versioncheck.go",
}

// Analyzer is the tenantcheck vet pass.
var Analyzer = &analysis.Analyzer{
	Name: "tenantcheck",
	Doc:  "flags raw SQL and joins that bypass tenant auto-scoping on tenant-scoped tables (ADR-0007)",
	Run:  run,
}

var skipFlag string

func init() {
	Analyzer.Flags.StringVar(&skipFlag, "skip", strings.Join(defaultSkipSuffixes, ","),
		"comma-separated file-path suffixes to skip wholesale (legitimately-raw files)")
}

func run(pass *analysis.Pass) (interface{}, error) {
	skips := parseSkips(skipFlag)

	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		// Test files are skipped: tests deliberately manipulate DB state
		// (raw inserts that violate constraints, cross-tenant fixtures) against
		// ephemeral test databases with no tenant context. The guard protects
		// production request paths, which the runtime gate also covers.
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		if skipped(filename, skips) {
			continue
		}

		allow := collectAllowLines(pass, file)

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			kind := methodKind(sel.Sel.Name)
			if kind == "" {
				return true
			}
			// Only *gorm.DB chains — this is what distinguishes db.Raw(...)
			// from unrelated Raw methods (e.g. the HTML templating elem.Raw).
			if !isGormDB(pass.TypesInfo.TypeOf(sel.X)) {
				return true
			}
			if len(call.Args) == 0 {
				return true
			}
			s, ok := constString(pass, call.Args[0])
			if !ok {
				return true // non-constant SQL/join string: cannot inspect, skip.
			}
			table := tenantTableRe.FindString(s)
			if table == "" {
				return true
			}

			line := pass.Fset.Position(call.Pos()).Line
			if allow[line] || allow[line-1] {
				return true
			}

			switch kind {
			case "raw":
				pass.Reportf(call.Pos(),
					"raw SQL references tenant-scoped table %q; bypasses tenant auto-scoping (ADR-0007 / CONTEXT decision 2) — route through the scoped query or annotate with //tenantcheck:allow <reason>",
					table)
			case "join":
				pass.Reportf(call.Pos(),
					"join references tenant-scoped table %q; the join bypasses single-model tenant scoping (ADR-0007 / CONTEXT decision 2) — scope the joined table explicitly or annotate with //tenantcheck:allow <reason>",
					table)
			}
			return true
		})
	}

	return nil, nil
}

// methodKind maps a gorm method name to a finding category, or "" if the method
// is not a bypass vector.
func methodKind(name string) string {
	switch name {
	case "Raw", "Exec":
		return "raw"
	case "Joins":
		return "join"
	default:
		return ""
	}
}

// isGormDB reports whether t is gorm.io/gorm.DB or *gorm.io/gorm.DB.
func isGormDB(t types.Type) bool {
	if t == nil {
		return false
	}
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	return obj.Pkg().Path() == "gorm.io/gorm" && obj.Name() == "DB"
}

// constString returns the constant string value of e (folding constant
// concatenations like `"ALTER TABLE " + name` when name is itself constant),
// and false when e is not a compile-time string constant.
func constString(pass *analysis.Pass, e ast.Expr) (string, bool) {
	tv, ok := pass.TypesInfo.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

// collectAllowLines returns the set of line numbers carrying a
// //tenantcheck:allow directive.
func collectAllowLines(pass *analysis.Pass, file *ast.File) map[int]bool {
	lines := make(map[int]bool)
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			text := c.Text
			text = strings.TrimPrefix(text, "//")
			text = strings.TrimPrefix(text, "/*")
			if strings.HasPrefix(strings.TrimSpace(text), "tenantcheck:allow") {
				lines[posLine(pass.Fset, c.Slash)] = true
			}
		}
	}
	return lines
}

func posLine(fset *token.FileSet, pos token.Pos) int {
	return fset.Position(pos).Line
}

func parseSkips(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func skipped(filename string, skips []string) bool {
	for _, suffix := range skips {
		if strings.HasSuffix(filename, suffix) {
			return true
		}
	}
	return false
}
