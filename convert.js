const fs = require("fs");

function convertToTargetFormat(inputJson) {
  const { entities, relationships } = inputJson;
  const result = [];

  // Process each entity
  for (const [entityName, entityData] of Object.entries(entities)) {
    const moduleName = entityName.toLowerCase();
    const fields = [];
    const relations = [];

    // Process fields
    for (const [fieldName, fieldData] of Object.entries(entityData.fields)) {
      if (fieldName.includes("_id")) {
        continue;
      }
      const field = {
        fieldName: convertToCamelCase(fieldName),
        fieldType: mapFieldType(fieldData.type),
      };

      // Add additional properties
      if (fieldData.primaryKey) {
        field.primary = true;
      }

      if (fieldData.unique) {
        field.unique = true;
      }

      if (fieldData.searchable) {
        field.searchable = true;
      }

      if (fieldData.nullable) {
        field.nullable = true;
      }
      if (fieldData.filterBy) {
        field.filterBy = true;
      }
      if (fieldData.virtual) {
        field.virtual = true;
      }

      fields.push(field);
    }

    // Find and process relationships for this entity
    relationships.forEach((rel) => {
      if (rel.from === entityName) {
        const relation = {
          relationType: mapRelationType(rel.type),
          relatedEntity: rel.to,
          fieldName: pascalToCamel(rel.name),
          nullable: false, // Default value, could be adjusted based on your rules
        };
        if (rel.type === "one-to-many") {
          const arr = rel.toField.split("_");
          relation.foreignKey = pascalToCamel(`${arr.slice(0, -1).join("_")}`);
        }
        if (rel.cascade) {
          relation.cascade = true;
        }
        if (rel.foreignKey) {
          relation.foreignKey = rel.foreignKey;
        }

        relations.push(relation);
      }
    });

    // Create the entity object in target format
    const convertedEntity = {
      entityName,
      moduleName,
      fields,
      relations,
    };

    result.push(convertedEntity);
  }

  return result;
}
function pascalToCamel(pascalString) {
  if (!pascalString || typeof pascalString !== "string") {
    return pascalString;
  }

  return pascalString.charAt(0).toLowerCase() + pascalString.slice(1);
}

function convertToCamelCase(str) {
  return str.replace(/_([a-z])/g, (g) => g[1].toUpperCase());
}

function mapFieldType(type) {
  const typeMap = {
    uuid: "string",
    string: "string",
    text: "string",
    number: "number",
    integer: "number",
    decimal: "number",
    boolean: "boolean",
    date: "date",
    timestamp: "date",
    point: "string",
    jsonb: "object",
    enum: "string",
    interval: "string",
  };

  return typeMap[type] || "string";
}

function mapRelationType(type) {
  const typeMap = {
    "one-to-one": "OneToOne",
    "one-to-many": "OneToMany",
    "many-to-one": "ManyToOne",
    "many-to-many": "ManyToMany",
  };

  return typeMap[type] || "OneToMany";
}

// Read input file
const inputFile = process.argv[2] || "input.json";
const outputFile = process.argv[3] || "output.json";

try {
  const input = JSON.parse(fs.readFileSync(inputFile, "utf8"));
  const convertedData = convertToTargetFormat(input);

  // Write each entity to a separate file or combine them as needed
  fs.writeFileSync(outputFile, JSON.stringify(convertedData, null, 2));
  console.log(
    `Conversion completed successfully. Output saved to ${outputFile}`,
  );
} catch (error) {
  console.error("Error during conversion:", error.message);
}
