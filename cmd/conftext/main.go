package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const CurrentVersion = "v0.3.3"

var (
	path       string
	markdown   bool
	env        bool
	yaml       bool
	prefix     string
	version    bool
	structName string
)

// Field represents a single field in the config structure
type Field struct {
	Name    string
	Type    string
	Comment string
}

// EnvVar represents an environment variable derived from a config field
type EnvVar struct {
	Chain []Field
}

func (e EnvVar) Path() string {
	parts := make([]string, 0, len(e.Chain)+1)
	if prefix != "" {
		parts = append(parts, prefix)
	}
	for _, field := range e.Chain {
		parts = append(parts, field.Name)
	}
	return strings.ToUpper(strings.Join(parts, "_"))
}

func (e EnvVar) YAMLPath() string {
	parts := make([]string, len(e.Chain))
	for i, field := range e.Chain {
		parts[i] = field.Name
	}
	return strings.Join(parts, ".")
}

func (e EnvVar) LastField() Field {
	if len(e.Chain) == 0 {
		return Field{}
	}
	return e.Chain[len(e.Chain)-1]
}

// isPrimitiveType returns true if the type is a primitive or string type
func isPrimitiveType(typeStr string) bool {
	primitives := map[string]bool{
		"string": true,
		"int":    true,
		"bool":   true,
	}
	return primitives[strings.TrimPrefix(typeStr, "*")]
}

// getYAMLTag extracts the yaml tag value from a field tag
func getYAMLTag(tag string) string {
	if tag == "" {
		return ""
	}
	tag = strings.Trim(tag, "`")
	for _, tagPart := range strings.Split(tag, " ") {
		if strings.HasPrefix(tagPart, "yaml:") {
			// Extract the yaml tag content
			content := strings.Trim(strings.Split(tagPart, ":")[1], "\"")
			// Split by comma and take the first part as the field name
			return strings.Split(content, ",")[0]
		}
	}
	return ""
}

// getTypeString returns a string representation of the type
func getTypeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + getTypeString(t.X)
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", t.X.(*ast.Ident).Name, t.Sel.Name)
	default:
		return fmt.Sprintf("%T", expr)
	}
}

// processStructFields recursively processes struct fields and builds environment variable paths
func processStructFields(field ast.Expr, chain []Field, vars *[]EnvVar) {
	switch t := field.(type) {
	case *ast.Ident:
		if isPrimitiveType(t.Name) {
			*vars = append(*vars, EnvVar{Chain: chain})
		} else if obj := t.Obj; obj != nil {
			if ts, ok := obj.Decl.(*ast.TypeSpec); ok {
				if st, ok := ts.Type.(*ast.StructType); ok {
					for _, f := range st.Fields.List {
						processField(f, chain, vars)
					}
				}
			}
		}
	case *ast.StarExpr:
		if isPrimitiveType(getTypeString(t.X)) {
			*vars = append(*vars, EnvVar{Chain: chain})
		} else {
			processStructFields(t.X, chain, vars)
		}
	case *ast.SelectorExpr:
		*vars = append(*vars, EnvVar{Chain: chain})
	case *ast.StructType:
		for _, f := range t.Fields.List {
			processField(f, chain, vars)
		}
	}
}

// processField handles a single struct field
func processField(field *ast.Field, parentChain []Field, vars *[]EnvVar) {
	if field.Names == nil {
		processStructFields(field.Type, parentChain, vars)
		return
	}

	yamlTag := getYAMLTag(field.Tag.Value)
	fieldName := yamlTag
	if fieldName == "" {
		fieldName = strings.ToLower(field.Names[0].Name)
	}

	// Get field comment
	var comment string
	if field.Doc != nil {
		comments := make([]string, 0, len(field.Doc.List))
		for _, c := range field.Doc.List {
			comments = append(comments, strings.TrimSpace(strings.TrimPrefix(c.Text, "//")))
		}
		comment = strings.Join(comments, " ")
	}

	newField := Field{
		Name:    fieldName,
		Type:    getTypeString(field.Type),
		Comment: comment,
	}
	chain := make([]Field, len(parentChain))
	copy(chain, parentChain)
	chain = append(chain, newField)

	processStructFields(field.Type, chain, vars)
}

// getEnvExampleValue returns an example value for environment variables based on the type
func getEnvExampleValue(fieldType string) string {
	baseType := strings.TrimPrefix(fieldType, "*")
	switch {
	case baseType == "string":
		return "string"
	case strings.HasPrefix(baseType, "int") || strings.HasPrefix(baseType, "uint"):
		return "integer"
	case strings.HasPrefix(baseType, "float"):
		return "number"
	case baseType == "bool":
		return "true/false"
	default:
		return "string"
	}
}

func printEnvText(vars []EnvVar) {
	fmt.Println("Environment variable paths:")
	fmt.Println("NAME                           VALUE           DESCRIPTION")
	fmt.Println("----                          -----           -----------")
	for _, v := range vars {
		lastField := v.LastField()
		if lastField.Comment != "" {
			fmt.Printf("%-30s %-15s // %s\n", v.Path(), getEnvExampleValue(lastField.Type), lastField.Comment)
		} else {
			fmt.Printf("%-30s %s\n", v.Path(), getEnvExampleValue(lastField.Type))
		}
	}
}

func printEnvMarkdown(vars []EnvVar) {
	fmt.Println("| Environment Variable | Expected Value | Description |")
	fmt.Println("|---------------------|----------------|-------------|")
	for _, v := range vars {
		lastField := v.LastField()
		comment := lastField.Comment
		if comment == "" {
			comment = "-"
		}
		fmt.Printf("| `%s` | `%s` | %s |\n", v.Path(), getEnvExampleValue(lastField.Type), comment)
	}
}

func printYAMLSample(vars []EnvVar) {
	printed := make(map[string]bool)
	for _, v := range vars {
		path := v.YAMLPath()
		parts := strings.Split(path, ".")

		// Print each level of nesting
		current := ""
		indent := ""
		for i, part := range parts {
			if i == len(parts)-1 {
				// Last part - print with a sample value based on type
				fmt.Printf("%s%s: %s\n", indent, part, getEnvExampleValue(v.LastField().Type))
			} else {
				if current != "" {
					current += "."
				}
				current += part
				if !printed[current] {
					fmt.Printf("%s%s:\n", indent, part)
					printed[current] = true
				}
				indent += "  "
			}
		}
	}
}

func main() {
	flag.StringVar(&path, "path", "", "path to the file to parse")
	flag.BoolVar(&markdown, "markdown", false, "output in markdown format")
	flag.BoolVar(&env, "env", false, "output environment variables")
	flag.BoolVar(&yaml, "yaml", false, "output yaml sample")
	flag.StringVar(&prefix, "prefix", "", "prefix for environment variables")
	flag.StringVar(&structName, "struct", "", "name of the struct to parse")
	flag.BoolVar(&version, "version", false, "print version and exit")
	flag.Parse()

	if version {
		fmt.Println(CurrentVersion)
		return
	}

	configStructName := "Config"
	if structName != "" {
		configStructName = structName
	}

	if path == "" {
		log.Fatal("path is required")
	}

	if yaml && env {
		log.Fatal("yaml and env flags cannot be used together")
	}

	if !yaml && !env {
		env = true // default to env output
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		log.Fatalf("failed to read directory %s: %v", path, err)
	}

	var configStruct *ast.StructType
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(path, entry.Name())
		fs := token.NewFileSet()
		node, err := parser.ParseFile(fs, filePath, nil, parser.ParseComments)
		if err != nil {
			continue
		}

		ast.Inspect(node, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok {
				if st, ok := ts.Type.(*ast.StructType); ok {
					if ts.Name.Name == configStructName {
						configStruct = st
						return false
					}
				}
			}
			return true
		})
		if configStruct != nil {
			break
		}
	}

	if configStruct == nil {
		log.Fatal("Config struct not found")
	}

	vars := make([]EnvVar, 0)
	for _, field := range configStruct.Fields.List {
		processField(field, nil, &vars)
	}

	if yaml {
		printYAMLSample(vars)
	} else if env {
		if markdown {
			printEnvMarkdown(vars)
		} else {
			printEnvText(vars)
		}
	}
}
