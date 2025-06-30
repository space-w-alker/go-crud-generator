# Go CRUD Generator

A Go code generator that creates CRUD operations, models, repositories, and controllers based on JSON entity definitions.

## Installation

```bash
go mod download
```

## Configuration

The generator can be configured using environment variables loaded from a `.env` file.

### Environment Variables

Create a `.env` file in the project root with the following variables:

```env
# Module name for the generated Go code
# This should match your Go module path
MODULE_NAME=github.com/space-w-alker/campus-nexus/internal/server

# You can add other configuration variables here as needed
# OUTPUT_DIR=output
# LOG_LEVEL=info
```

### Required Variables

- `MODULE_NAME`: The Go module name for the generated code (e.g., `github.com/your-org/your-project/internal/server`)

If no `.env` file is found or `MODULE_NAME` is not set, the generator will use the default value: `github.com/space-w-alker/campus-nexus/internal/server`

## Usage

```bash
# Build the generator
go build -o generator main.go

# Run with input JSON file
./generator input.json [output_directory]

# Or run directly with go
go run main.go input.json [output_directory]
```

## Input Format

The generator expects a JSON file containing entity definitions. See `input.json` for an example.

## Output

The generator creates the following directory structure:

```
output/
├── models/
├── controllers/
├── repositories/
├── dto/
├── middleware/
├── errs/
└── wire.go
```

## Example

1. Copy `.env.example` to `.env` and update the `MODULE_NAME`:
   ```bash
   cp .env.example .env
   ```

2. Edit `.env` to set your module name:
   ```env
   MODULE_NAME=github.com/your-org/your-project/internal/server
   ```

3. Run the generator:
   ```bash
   go run main.go input.json output
   ``` 