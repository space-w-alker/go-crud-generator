package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"
)

// Input types matching your TypeScript interface
type Field struct {
	FieldName string      `json:"fieldName"`
	FieldType string      `json:"fieldType"`
	Primary   bool        `json:"primary"`
	Nullable  bool        `json:"nullable"`
	Default   interface{} `json:"default"`
	Unique    bool        `json:"unique"`
}

type Relation struct {
	RelationType   string `json:"relationType"`
	RelatedEntity  string `json:"relatedEntity"`
	FieldName      string `json:"fieldName"`
	Nullable       bool   `json:"nullable"`
	Cascade        bool   `json:"cascade"`
	DeleteBehavior string `json:"deleteBehavior"`
}

type QueryParameter struct {
	ParamName   string `json:"paramName"`
	ParamType   string `json:"paramType"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type CustomEndpoint struct {
	EndpointName string `json:"endpointName"`
	HTTPMethod   string `json:"httpMethod"`
	Path         string `json:"path"`
	Description  string `json:"description"`
}

type AdditionalFeatures struct {
	SoftDelete             bool     `json:"softDelete"`
	Pagination             bool     `json:"pagination"`
	Sorting                bool     `json:"sorting"`
	DateFiltering          bool     `json:"dateFiltering"`
	AuthenticationRequired bool     `json:"authenticationRequired"`
	CustomValidationRules  []string `json:"customValidationRules"`
}

type InputJson struct {
	EntityName         string             `json:"entityName"`
	ModuleName         string             `json:"moduleName"`
	TableName          string             `json:"tableName,omitempty"`
	Fields             []Field            `json:"fields"`
	Relations          []Relation         `json:"relations"`
	QueryParameters    []QueryParameter   `json:"queryParameters"`
	AdditionalFeatures AdditionalFeatures `json:"additionalFeatures"`
	CustomEndpoints    []CustomEndpoint   `json:"customEndpoints"`
}

// Helper functions for templates
func (input *InputJson) GetTableName() string {
	if input.TableName != "" {
		return input.TableName
	}
	return strings.ToLower(input.EntityName) + "s"
}

func (input *InputJson) EntityNameLower() string {
	return strings.ToLower(input.EntityName)
}

func (input *InputJson) EntityNamePlural() string {
	return input.EntityName + "s"
}

func (input *InputJson) HasPrimaryKey() bool {
	for _, field := range input.Fields {
		if field.Primary {
			return true
		}
	}
	return false
}

func (input *InputJson) GetPrimaryKey() Field {
	for _, field := range input.Fields {
		if field.Primary {
			return field
		}
	}
	// Default to ID if no primary key is specified
	return Field{FieldName: "ID", FieldType: "uint", Primary: true, Nullable: false}
}

func (input *InputJson) GetPrimaryKeyName() string {
	return input.GetPrimaryKey().FieldName
}

func (input *InputJson) GetPrimaryKeyType() string {
	return convertTypeScriptTypeToGo(input.GetPrimaryKey().FieldType)
}

// Helper function to convert TypeScript types to Go types
func convertTypeScriptTypeToGo(tsType string) string {
	switch strings.ToLower(tsType) {
	case "string":
		return "string"
	case "number", "int", "integer":
		return "int"
	case "float", "double":
		return "float64"
	case "boolean", "bool":
		return "bool"
	case "date", "datetime":
		return "time.Time"
	case "uuid":
		return "string"
	case "uint", "uint64":
		return "uint"
	default:
		return "interface{}"
	}
}

// Helper to convert field name to a Go-style name
func toGoFieldName(name string) string {
	if len(name) == 0 {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func toSnakeCase(s string) string {
	var result strings.Builder

	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 && !unicode.IsUpper(rune(s[i-1])) {
				result.WriteRune('_')
			} else if i > 0 && i < len(s)-1 && unicode.IsUpper(rune(s[i-1])) && !unicode.IsUpper(rune(s[i+1])) {
				result.WriteRune('_')
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// Template helpers
var templateFuncs = template.FuncMap{
	"toGoFieldName":             toGoFieldName,
	"lower":                     func() string { return "" },
	"convertTypeScriptTypeToGo": convertTypeScriptTypeToGo,
	"formatGormTags": func(field Field, tableName string) string {
		var tags []string
		column := strings.ToLower(field.FieldName)

		tags = append(tags, fmt.Sprintf("column:%s", column))

		if field.Primary {
			tags = append(tags, "primaryKey")
		}

		if !field.Nullable && !field.Primary {
			tags = append(tags, "not null")
		}

		if field.Unique {
			tags = append(tags, "unique")
		}

		if field.Default != nil && field.Default != "" {
			tags = append(tags, fmt.Sprintf("default:%v", field.Default))
		}

		// Auto-increment for primary keys that are integers
		if field.Primary && (strings.Contains(field.FieldType, "int") || strings.Contains(field.FieldType, "uint")) {
			tags = append(tags, "autoIncrement")
		}

		return fmt.Sprintf("gorm:\"%s\"", strings.Join(tags, ";"))
	},
	"formatValidationTags": func(field Field) string {
		var tags []string

		if !field.Nullable && !field.Primary {
			tags = append(tags, "required")
		}

		return fmt.Sprintf("validate:\"%s\"", strings.Join(tags, ","))
	},
	"formatValidationRules": func(field Field) string {
		var rules []string

		if !field.Nullable && !field.Primary {
			rules = append(rules, "required")
		}

		return strings.Join(rules, ",")
	},
	"formatRelation": func(relation Relation) string {
		switch relation.RelationType {
		case "OneToOne", "ManyToOne":
			// Add both the foreign key field and the relationship field with a newline
			foreignKeyField := fmt.Sprintf("%sId uint `gorm:\"column:%s_id\"`",
				toGoFieldName(relation.FieldName),
				toSnakeCase(relation.FieldName))

			relationField := fmt.Sprintf("%s *%s `gorm:\"foreignKey:%s\"`",
				toGoFieldName(relation.FieldName),
				relation.RelatedEntity,
				toGoFieldName(relation.FieldName)+"Id")

			return foreignKeyField + "\n\t" + relationField

		case "OneToMany":
			return fmt.Sprintf("%s []%s `gorm:\"foreignKey:%sId\"`",
				toGoFieldName(relation.FieldName),
				relation.RelatedEntity,
				strings.TrimSuffix(relation.FieldName, "s"))

		case "ManyToMany":
			return fmt.Sprintf("%s []%s `gorm:\"many2many:%s_%s\"`",
				toGoFieldName(relation.FieldName),
				relation.RelatedEntity,
				strings.ToLower(relation.RelatedEntity),
				strings.ToLower(strings.TrimSuffix(relation.FieldName, "s")))

		default:
			return ""
		}
	},
	"formatRelationDTO": func(relation Relation) string {
		switch relation.RelationType {
		case "OneToOne", "ManyToOne":
			foreignKeyField := fmt.Sprintf("%sId uint `json:\"%sId,omitempty\"`",
				toGoFieldName(relation.FieldName),
				relation.FieldName)
			relationField := fmt.Sprintf("%s *%sResponse `json:\"%s,omitempty\"`",
				toGoFieldName(relation.FieldName),
				relation.RelatedEntity,
				relation.FieldName)
			return foreignKeyField + "\n\t" + relationField
		case "OneToMany", "ManyToMany":
			return fmt.Sprintf("%s []%sResponse `json:\"%s,omitempty\"`",
				toGoFieldName(relation.FieldName),
				relation.RelatedEntity,
				relation.FieldName)
		default:
			return ""
		}
	},
	"relatedIDType": func(relation Relation) string {
		return "uint"
	},
	"getZeroValue": func(typeName string) string {
		switch strings.ToLower(typeName) {
		case "string":
			return "\"\""
		case "int", "uint", "int64", "uint64", "float", "float64":
			return "0"
		case "bool":
			return "false"
		case "time.time":
			return "time.Time{}"
		default:
			return "nil"
		}
	},
}

func main() {
	// Read the input JSON from a file or command-line argument
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run generator.go <input.json> [output_directory]")
		return
	}

	inputFile := os.Args[1]
	outputDir := "output"
	if len(os.Args) > 2 {
		outputDir = os.Args[2]
	}

	// Parse input file and create output directories
	entities, err := parseInputFile(inputFile)
	if err != nil {
		fmt.Printf("Error processing input: %v\n", err)
		return
	}

	// Create base output directory
	if err := createOutputDirectories(outputDir); err != nil {
		fmt.Printf("Error creating output directories: %v\n", err)
		return
	}

	// Determine module name (for imports)
	moduleName := "github.com/space-w-alker/myapp"

	// Generate code for each entity
	for _, entity := range entities {
		if err := generateEntityCode(entity, outputDir, moduleName); err != nil {
			fmt.Printf("Error generating code for entity %s: %v\n", entity.EntityName, err)
		} else {
			fmt.Printf("Generated code for %s in %s\n", entity.EntityName, outputDir)
		}
	}
}

// parseInputFile reads and parses the input JSON file
func parseInputFile(inputFile string) ([]InputJson, error) {
	// Read input file
	inputData, err := os.ReadFile(inputFile)
	if err != nil {
		return nil, fmt.Errorf("error reading input file: %v", err)
	}

	// Parse JSON
	var entities []InputJson
	err = json.Unmarshal(inputData, &entities)
	if err != nil {
		// Try parsing as a single entity
		var singleEntity InputJson
		if err = json.Unmarshal(inputData, &singleEntity); err != nil {
			return nil, fmt.Errorf("error parsing JSON: %v", err)
		}
		entities = []InputJson{singleEntity}
	}

	return entities, nil
}

// createOutputDirectories creates the necessary directory structure
func createOutputDirectories(outputDir string) error {
	dirs := []string{
		filepath.Join(outputDir, "models"),
		filepath.Join(outputDir, "controllers"),
		filepath.Join(outputDir, "repositories"),
		filepath.Join(outputDir, "dto"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("error creating directory %s: %v", dir, err)
		}
	}

	return nil
}

// generateEntityCode generates code files for a single entity
func generateEntityCode(entity InputJson, outputDir, moduleName string) error {
	// Create template data with all necessary fields
	templateData := struct {
		*InputJson
		ModuleName      string
		EntityNameLower string
	}{
		InputJson:       &entity,
		ModuleName:      moduleName,
		EntityNameLower: strings.ToLower(entity.EntityName),
	}

	// Define file templates
	templates := map[string]string{
		filepath.Join(outputDir, "models", strings.ToLower(entity.EntityName)+".go"):            filepath.Join("templates", "model.tmpl"),
		filepath.Join(outputDir, "dto", strings.ToLower(entity.EntityName)+"_base.go"):          filepath.Join("templates", "dto.tmpl"),
		filepath.Join(outputDir, "repositories", strings.ToLower(entity.EntityName)+"_base.go"): filepath.Join("templates", "repository.tmpl"),
		filepath.Join(outputDir, "controllers", strings.ToLower(entity.EntityName)+"_base.go"):  filepath.Join("templates", "controller.tmpl"),
	}

	// Generate each file from its template
	for filePath, templateContent := range templates {
		if err := generateFileFromTemplate(filePath, templateContent, templateData); err != nil {
			return fmt.Errorf("error generating file %s: %v", filePath, err)
		}
	}

	return nil
}

// generateFileFromTemplate creates a file from a template with the given data
func generateFileFromTemplate(filePath, templatePath string, data interface{}) error {
	// Read the template file
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("error reading template file %s: %v", templatePath, err)
	}

	// Parse the template
	tmpl, err := template.New(filepath.Base(filePath)).Funcs(templateFuncs).Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("error parsing template: %v", err)
	}

	// Create the file
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer file.Close()

	// Execute the template
	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("error executing template: %v", err)
	}

	return nil
}
