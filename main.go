package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"github.com/samber/lo"
)

// Input types matching your TypeScript interface
type Field struct {
	FieldName string      `json:"fieldName"`
	FieldType string      `json:"fieldType"`
	FilterBy  bool        `json:"filterBy,omitempty"`
	Primary   bool        `json:"primary"`
	Nullable  bool        `json:"nullable"`
	Default   interface{} `json:"default"`
	Unique    bool        `json:"unique"`
}

type Relation struct {
	RelationType   string `json:"relationType"`
	RelatedEntity  string `json:"relatedEntity"`
	ForeignKey     string `json:"foreignKey"`
	FieldName      string `json:"fieldName"`
	Nullable       bool   `json:"nullable"`
	Cascade        bool   `json:"cascade"`
	DeleteBehavior string `json:"deleteBehavior"`
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

type Entity struct {
	EntityName         string             `json:"entityName"`
	ModuleName         string             `json:"moduleName"`
	TableName          string             `json:"tableName,omitempty"`
	Fields             []Field            `json:"fields"`
	Relations          []Relation         `json:"relations"`
	AdditionalFeatures AdditionalFeatures `json:"additionalFeatures"`
	CustomEndpoints    []CustomEndpoint   `json:"customEndpoints"`
}

// Helper functions for templates
func (input *Entity) GetTableName() string {
	if input.TableName != "" {
		return input.TableName
	}
	return lo.SnakeCase(input.EntityName) + "s"
}

func (input *Entity) EntityNameLower() string {
	return strings.ToLower(input.EntityName)
}

func (input *Entity) EntityNamePlural() string {
	return input.EntityName + "s"
}

func (input *Entity) HasPrimaryKey() bool {
	for _, field := range input.Fields {
		if field.Primary {
			return true
		}
	}
	return false
}

func (input *Entity) GetPrimaryKey() Field {
	for _, field := range input.Fields {
		if field.Primary {
			return field
		}
	}
	// Default to ID if no primary key is specified
	return Field{FieldName: "ID", FieldType: "string", Primary: true, Nullable: false}
}

func (input *Entity) GetPrimaryKeyName() string {
	return input.GetPrimaryKey().FieldName
}

func (input *Entity) GetPrimaryKeyType() string {
	return convertTypeScriptTypeToGo(input.GetPrimaryKey().FieldType)
}

// Helper function to convert TypeScript types to Go types
func convertTypeScriptTypeToGo(tsType string) string {
	switch strings.ToLower(tsType) {
	case "string", "enum", "json":
		return "string"
	case "number", "int", "integer":
		return "int"
	case "float", "double", "decimal":
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
	"snakeCase":                 lo.SnakeCase,
	"pascalCase":                lo.PascalCase,
	"camelCase":                 lo.CamelCase,
	"convertTypeScriptTypeToGo": convertTypeScriptTypeToGo,
	"formatGormTags": func(field Field, tableName string) string {
		var tags []string
		column := lo.SnakeCase(field.FieldName)

		tags = append(tags, fmt.Sprintf("column:%s", column))

		if field.Primary {
			tags = append(tags, "primaryKey", "type:char(36)")
		}

		if !field.Nullable && !field.Primary {
			tags = append(tags, "not null")
		}

		if field.Unique {
			tags = append(tags, "unique")
		}

		if field.Default != nil && field.Default != "" && !field.Primary {
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
	"formatRelation": func(entityName string, relation Relation) string {
		switch relation.RelationType {
		case "OneToOne", "ManyToOne":
			// Add both the foreign key field and the relationship field with a newline
			foreignKeyField := fmt.Sprintf("%sID *string `gorm:\"column:%s_id\"`",
				toGoFieldName(relation.FieldName),
				toSnakeCase(relation.FieldName))

			relationField := fmt.Sprintf("%s *%s `gorm:\"foreignKey:%s\"`",
				toGoFieldName(relation.FieldName),
				relation.RelatedEntity,
				toGoFieldName(relation.FieldName)+"ID")

			return foreignKeyField + "\n\t" + relationField

		case "OneToMany":
			return fmt.Sprintf("%s []%s `gorm:\"foreignKey:%sID\"`",
				toGoFieldName(relation.FieldName),
				relation.RelatedEntity,
				lo.CoalesceOrEmpty(lo.PascalCase(relation.ForeignKey), lo.PascalCase(entityName)))

		case "ManyToMany":
			foreignTable := fmt.Sprintf("%s_%s", strings.ToLower(entityName), strings.ToLower(relation.RelatedEntity))
			return fmt.Sprintf("%s []%s `gorm:\"many2many:%s\"`",
				toGoFieldName(relation.FieldName),
				relation.RelatedEntity,
				lo.CoalesceOrEmpty(relation.ForeignKey, foreignTable))

		default:
			return ""
		}
	},
	"formatRelationDTO": func(relation Relation) string {
		switch relation.RelationType {
		case "OneToOne", "ManyToOne":
			foreignKeyField := fmt.Sprintf("%sID *string `json:\"%sID,omitempty\"`",
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
		return "string"
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

	fmt.Printf("%v\n\n", strings.Join(lo.Map(entities, func(item Entity, index int) string { return item.EntityName }), ","))

	AssignRelations(entities)

	// Create base output directory
	if err := createOutputDirectories(outputDir); err != nil {
		fmt.Printf("Error creating output directories: %v\n", err)
		return
	}

	// Determine module name (for imports)
	moduleName := "github.com/space-w-alker/campus-nexus/internal/server"

	if err := generateGenericCode(outputDir, moduleName, entities); err != nil {
		fmt.Printf("Error generating generic code: %v", err)
	}

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
func parseInputFile(inputFile string) ([]Entity, error) {
	// Read input file
	inputData, err := os.ReadFile(inputFile)
	if err != nil {
		return nil, fmt.Errorf("error reading input file: %v", err)
	}

	// Parse JSON
	var entities []Entity
	err = json.Unmarshal(inputData, &entities)
	if err != nil {
		// Try parsing as a single entity
		var singleEntity Entity
		if err = json.Unmarshal(inputData, &singleEntity); err != nil {
			return nil, fmt.Errorf("error parsing JSON: %v", err)
		}
		entities = []Entity{singleEntity}
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
		filepath.Join(outputDir, "middleware"),
		filepath.Join(outputDir, "errs"),
		filepath.Join(outputDir, "errs", "errcodes"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("error creating directory %s: %v", dir, err)
		}
	}

	return nil
}

func generateGenericCode(outputDir string, moduleName string, data []Entity) error {
	temp := strings.Split(moduleName, "/")
	packageName := temp[len(temp)-1]
	d := struct {
		Entities    []Entity
		PackageName string
		ModuleName  string
	}{Entities: data, PackageName: packageName, ModuleName: moduleName}
	if err := generateFileFromTemplate(path.Join(outputDir, "dto", "utils.go"), path.Join("templates", "dto_utils.tmpl"), struct{}{}, true); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "repositories", "utils.go"), path.Join("templates", "repository_utils.tmpl"), struct{}{}, true); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "routes.go"), path.Join("templates", "routes.tmpl"), d, false); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "database.go"), path.Join("templates", "database.tmpl"), d, false); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "server.go"), path.Join("templates", "server.tmpl"), d, false); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "repositories", "auth_service.go"), path.Join("templates", "auth_service.tmpl"), d, true); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "controllers", "auth_controller.go"), path.Join("templates", "auth_controller.tmpl"), d, true); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "errs", "errs.go"), path.Join("templates", "errs.tmpl"), d, false); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "errs/errcodes", "errcodes.go"), path.Join("templates", "errcodes.tmpl"), d, false); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "middleware", "auth_middleware.go"), path.Join("templates", "middleware.tmpl"), d, false); err != nil {
		return err
	}
	return nil
}

// generateEntityCode generates code files for a single entity
func generateEntityCode(entity Entity, outputDir, moduleName string) error {
	// Create template data with all necessary fields
	templateData := struct {
		*Entity
		ModuleName      string
		EntityNameLower string
	}{
		Entity:          &entity,
		ModuleName:      moduleName,
		EntityNameLower: strings.ToLower(entity.EntityName),
	}

	// Define file templates
	templates := map[string]string{
		filepath.Join(outputDir, "models", lo.SnakeCase(entity.EntityName)+".go"):            filepath.Join("templates", "model.tmpl"),
		filepath.Join(outputDir, "dto", lo.SnakeCase(entity.EntityName)+"_base.go"):          filepath.Join("templates", "dto.tmpl"),
		filepath.Join(outputDir, "repositories", lo.SnakeCase(entity.EntityName)+"_base.go"): filepath.Join("templates", "repository.tmpl"),
		filepath.Join(outputDir, "controllers", lo.SnakeCase(entity.EntityName)+"_base.go"):  filepath.Join("templates", "controller.tmpl"),
	}

	if !fileExists(filepath.Join(outputDir, "controllers", lo.SnakeCase(entity.EntityName)+".go")) {
		templates[filepath.Join(outputDir, "controllers", lo.SnakeCase(entity.EntityName)+".go")] = filepath.Join("templates", "main.controller.tmpl")
	}

	// Generate each file from its template
	for filePath, templateContent := range templates {
		if err := generateFileFromTemplate(filePath, templateContent, templateData, false); err != nil {
			return fmt.Errorf("error generating file %s: %v", filePath, err)
		}
	}

	return nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil || !os.IsNotExist(err)
}

// generateFileFromTemplate creates a file from a template with the given data
func generateFileFromTemplate(filePath, templatePath string, data interface{}, skipExists bool) error {
	// Read the template file

	if skipExists && fileExists(filePath) {
		return nil
	}

	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("error reading template file %s: %v", templatePath, err)
	}

	// Parse the template
	tmpl, err := template.New(filepath.Base(filePath)).Funcs(templateFuncs).Parse(string(templateContent))
	if err != nil {
		return fmt.Errorf("error parsing template: %v", err)
	}

	// Execute the template to a buffer first
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("error executing template: %v", err)
	}

	// Format the Go code
	formattedSource, err := format.Source(buf.Bytes())
	if err != nil {
		// If formatting fails, we can either return the error or proceed with unformatted code
		// Here we choose to return the error
		return fmt.Errorf("error formatting output: %v", err)
	}

	// Create the file
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer file.Close()

	// Write the formatted content to the file
	if _, err := file.Write(formattedSource); err != nil {
		return fmt.Errorf("error writing to file: %v", err)
	}

	return nil
}

func AssignRelations(entities []Entity) {
	for i := range entities {
		entity := &entities[i]
		for j := range entity.Relations {
			relation := &entity.Relations[j]
			if relation.ForeignKey != "" {
				continue
			}
			for k := range entities {
				relEntity := &entities[k]
				if relation.RelatedEntity == relEntity.EntityName {
					for l := range relEntity.Relations {
						relEntityRelation := &relEntity.Relations[l]
						if relEntityRelation.RelatedEntity == entity.EntityName {
							relation.ForeignKey = relEntityRelation.FieldName
							break
						}
					}
				}
			}
		}
	}
}
