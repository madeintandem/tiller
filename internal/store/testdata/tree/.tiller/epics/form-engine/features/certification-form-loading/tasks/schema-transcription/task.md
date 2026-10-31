# Transcribe FORM-A-100 (2025) into JSON Schema

## Approach

Read the 2025 form PDF field by field and emit a JSON Schema with the
computed-field bindings the calculator reads.

## Surfaces

- `app/models/form_definitions/`

## Tests

- `spec/form_definitions/form_a_100_spec.rb`
