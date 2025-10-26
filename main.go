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

	"github.com/jinzhu/inflection"
	"github.com/joho/godotenv"
	"github.com/samber/lo"
)

// Input types matching your TypeScript interface
type Field struct {
	FieldName  string      `json:"fieldName"`
	Virtual    bool        `json:"virtual"`
	FieldType  string      `json:"fieldType"`
	FilterBy   bool        `json:"filterBy,omitempty"`
	Searchable bool        `json:"searchable,omitempty"`
	Primary    bool        `json:"primary"`
	Nullable   bool        `json:"nullable"`
	Default    interface{} `json:"default"`
	Unique     bool        `json:"unique"`
}

type Relation struct {
	RelationType  string `json:"relationType"`
	RelatedEntity string `json:"relatedEntity"`
	ForeignKey    string `json:"foreignKey"`
	FieldName     string `json:"fieldName"`
	Nullable      bool   `json:"nullable"`
	Cascade       bool   `json:"cascade"`
	OneToOneOwner bool   `json:"oneToOneOwner"`
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
	w := strings.Split(lo.SnakeCase(input.EntityName), "_")
	w[len(w)-1] = inflection.Plural(w[len(w)-1])
	return strings.Join(w, "_")
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
	"pluralize":                 inflection.Plural,
	"lower":                     strings.ToLower,
	"toLower":                   strings.ToLower,
	"snakeCase":                 lo.SnakeCase,
	"pascalCase":                lo.PascalCase,
	"camelCase":                 lo.CamelCase,
	"convertTypeScriptTypeToGo": convertTypeScriptTypeToGo,
	"formatGormTags": func(field Field, tableName string) string {
		var tags []string
		column := lo.SnakeCase(field.FieldName)

		tags = append(tags, fmt.Sprintf("column:%s", column))

		if field.Primary {
			tags = append(tags, "primaryKey", "type:char(36)", "not null")
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

			if !relation.OneToOneOwner {
				relation.ForeignKey = ""
			}

			relationField := fmt.Sprintf("%s *%s `gorm:\"foreignKey:%s\"`",
				toGoFieldName(relation.FieldName),
				relation.RelatedEntity,
				lo.CoalesceOrEmpty(relation.ForeignKey, toGoFieldName(relation.FieldName)+"ID"))

			if relation.OneToOneOwner {
				return relationField
			}
			return foreignKeyField + "\n\t" + relationField

		case "OneToMany":
			return fmt.Sprintf("%s []%s `gorm:\"foreignKey:%sID%s\"`",
				toGoFieldName(relation.FieldName),
				relation.RelatedEntity,
				lo.CoalesceOrEmpty(lo.PascalCase(relation.ForeignKey), lo.PascalCase(entityName)),
				func() string {
					if relation.Cascade {
						return ";constraint:OnDelete:CASCADE,OnUpdate:CASCADE"
					}
					return ";constraint:OnDelete:SET NULL,OnUpdate:SET NULL"
				}())

		case "ManyToMany":
			foreignTable := fmt.Sprintf("%s_%s", strings.ToLower(entityName), strings.ToLower(relation.RelatedEntity))
			return fmt.Sprintf("%s []%s `gorm:\"many2many:%s\"`",
				toGoFieldName(relation.FieldName),
				relation.RelatedEntity,
				lo.CoalesceOrEmpty(relation.ForeignKey, foreignTable)+";constraint:OnDelete:CASCADE,OnUpdate:CASCADE")

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
			if relation.OneToOneOwner {
				return relationField
			}
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

	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		// .env file is optional, so we don't fail if it doesn't exist
		fmt.Println("Warning: .env file not found, using default values")
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

	fmt.Printf("%v\n\n", strings.Join(lo.Map(entities, func(item Entity, index int) string { return item.EntityName }), ","))

	// Create base output directory
	if err := createOutputDirectories(outputDir); err != nil {
		fmt.Printf("Error creating output directories: %v\n", err)
		return
	}

	// Determine module name (for imports)
	moduleName := os.Getenv("MODULE_NAME")
	if moduleName == "" {
		moduleName = "github.com/space-w-alker/campus-nexus/internal/server"
	}

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
	if err := generateFileFromTemplate(path.Join(outputDir, "repositories", "utils.go"), path.Join("templates", "repository_utils.tmpl"), d, true); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "models", "utils.go"), path.Join("templates", "model_utils.tmpl"), struct{}{}, true); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "wire.go"), path.Join("templates", "wire.tmpl"), d, true); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "database.go"), path.Join("templates", "database.tmpl"), d, false); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "server.go"), path.Join("templates", "server.tmpl"), d, true); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "repositories", "auth_service.go"), path.Join("templates", "auth_service.tmpl"), d, true); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "controllers", "auth_controller.go"), path.Join("templates", "auth_controller.tmpl"), d, true); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "errs", "errs.go"), path.Join("templates", "errs.tmpl"), d, true); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "errs/errcodes", "errcodes.go"), path.Join("templates", "errcodes.tmpl"), d, true); err != nil {
		return err
	}
	if err := generateFileFromTemplate(path.Join(outputDir, "middleware", "auth_middleware.go"), path.Join("templates", "middleware.tmpl"), d, true); err != nil {
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

	modelPath := filepath.Join(outputDir, "models", lo.SnakeCase(entity.EntityName)+".go")
	modelTempPath := filepath.Join("templates", "model.tmpl")
	if err := generateFileFromTemplate(modelPath, modelTempPath, templateData, false); err != nil {
		return fmt.Errorf("error generating file %s: %v", modelPath, err)
	}

	baseDtoPath := filepath.Join(outputDir, "dto", lo.SnakeCase(entity.EntityName)+"_base.go")
	baseDtoTempPath := filepath.Join("templates", "dto_base.tmpl")
	if err := generateFileFromTemplate(baseDtoPath, baseDtoTempPath, templateData, false); err != nil {
		return fmt.Errorf("error generating file %s: %v", baseDtoPath, err)
	}

	dtoPath := filepath.Join(outputDir, "dto", lo.SnakeCase(entity.EntityName)+".go")
	dtoTempPath := filepath.Join("templates", "dto.tmpl")
	if err := generateFileFromTemplate(dtoPath, dtoTempPath, templateData, true); err != nil {
		return fmt.Errorf("error generating file %s: %v", dtoPath, err)
	}

	baseRepositoryPath := filepath.Join(outputDir, "repositories", lo.SnakeCase(entity.EntityName)+"_base.go")
	baseRepositoryTempPath := filepath.Join("templates", "repository_base.tmpl")
	if err := generateFileFromTemplate(baseRepositoryPath, baseRepositoryTempPath, templateData, false); err != nil {
		return fmt.Errorf("error generating file %s: %v", baseRepositoryPath, err)
	}

	repositoryPath := filepath.Join(outputDir, "repositories", lo.SnakeCase(entity.EntityName)+".go")
	repositoryTempPath := filepath.Join("templates", "repository.tmpl")
	if err := generateFileFromTemplate(repositoryPath, repositoryTempPath, templateData, true); err != nil {
		return fmt.Errorf("error generating file %s: %v", repositoryPath, err)
	}

	baseControllersPath := filepath.Join(outputDir, "controllers", lo.SnakeCase(entity.EntityName)+"_base.go")
	baseControllersTempPath := filepath.Join("templates", "controller_base.tmpl")
	if err := generateFileFromTemplate(baseControllersPath, baseControllersTempPath, templateData, false); err != nil {
		return fmt.Errorf("error generating file %s: %v", baseControllersPath, err)
	}

	controllerPath := filepath.Join(outputDir, "controllers", lo.SnakeCase(entity.EntityName)+".go")
	controllerTempPath := filepath.Join("templates", "controller.tmpl")
	if err := generateFileFromTemplate(controllerPath, controllerTempPath, templateData, true); err != nil {
		return fmt.Errorf("error generating file %s: %v", controllerPath, err)
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

// TopologicalSortEntities sorts entities by their dependencies
// Entities with no dependencies will be first in the returned slice
// Entities with dependencies will follow their dependencies
func TopologicalSortEntities(entities []Entity) ([]Entity, error) {
	// Create a graph representation of dependencies
	graph := make(map[string][]string)
	// Track in-degree (number of dependencies) for each entity
	inDegree := make(map[string]int)

	// Initialize maps with all entity names
	for _, entity := range entities {
		graph[entity.EntityName] = []string{}
		inDegree[entity.EntityName] = 0
	}

	// Build the dependency graph and in-degree counts
	for _, entity := range entities {
		for _, relation := range entity.Relations {
			// Only consider ManyToOne relationships as dependencies
			if relation.RelationType == "ManyToOne" {
				// This entity depends on the related entity
				graph[entity.EntityName] = append(graph[entity.EntityName], relation.RelatedEntity)
				inDegree[relation.RelatedEntity]++
			}
		}
	}

	// Queue for entities with no dependencies (in-degree of 0)
	var queue []string
	for entityName, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, entityName)
		}
	}

	// Result will store the sorted entity names
	var sortedNames []string

	// Process queue
	for len(queue) > 0 {
		// Take first entity from queue
		current := queue[0]
		queue = queue[1:]

		// Add to sorted result
		sortedNames = append(sortedNames, current)

		// For each entity that depends on the current entity
		for _, dependent := range graph[current] {
			// Reduce in-degree by 1
			inDegree[dependent]--

			// If in-degree becomes 0, add to queue
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	// Check for cycles (if we couldn't process all entities)
	if len(sortedNames) != len(entities) {
		// Find and report specific cycles
		cycles := findCycles(graph)
		return nil, fmt.Errorf("cycle(s) detected in entity dependencies: %v", cycles)
	}

	// Create result slice in correct order
	result := make([]Entity, len(entities))
	entityMap := make(map[string]Entity)

	// Create map for easy lookup
	for _, entity := range entities {
		entityMap[entity.EntityName] = entity
	}

	// Populate result slice in sorted order
	for i, name := range sortedNames {
		result[i] = entityMap[name]
	}

	return result, nil
}

// findCycles detects cycles in the dependency graph and returns them as strings
func findCycles(graph map[string][]string) []string {
	// Track visited nodes in current DFS path
	visited := make(map[string]bool)
	// Track nodes in the current recursion stack
	inStack := make(map[string]bool)
	// Store detected cycles
	var cycles []string

	// Temporary path for building cycle strings
	var currentPath []string

	// DFS function to detect cycles
	var dfs func(node string) bool
	dfs = func(node string) bool {
		// Mark current node as visited and add to recursion stack
		visited[node] = true
		inStack[node] = true
		currentPath = append(currentPath, node)

		// Check all dependencies
		for _, dependency := range graph[node] {
			// If not visited, recurse
			if !visited[dependency] {
				if dfs(dependency) {
					return true // Cycle found in subtree
				}
			} else if inStack[dependency] {
				// If the dependency is in recursion stack, we found a cycle
				// Find the start of the cycle in the current path
				cycleStart := -1
				for i, n := range currentPath {
					if n == dependency {
						cycleStart = i
						break
					}
				}

				// Build the cycle string
				if cycleStart != -1 {
					cyclePath := append([]string{}, currentPath[cycleStart:]...)
					cyclePath = append(cyclePath, dependency) // Close the loop
					cycles = append(cycles, strings.Join(cyclePath, " → "))
				}
				return true
			}
		}

		// Remove the current node from recursion stack and path
		inStack[node] = false
		currentPath = currentPath[:len(currentPath)-1]
		return false
	}

	// Check for cycles starting from each unvisited node
	for node := range graph {
		if !visited[node] {
			dfs(node)
		}
	}

	return cycles
}
