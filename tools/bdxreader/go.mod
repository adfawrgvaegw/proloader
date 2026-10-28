module proloader.local/bdxreader

go 1.25.0

require (
	github.com/Yeah114/bdump v0.0.0-00010101000000-000000000000
	github.com/Yeah114/blocks v0.0.0-20251025181709-54ab0b294dfe
	github.com/andybalholm/brotli v1.1.1
)

require github.com/google/uuid v1.6.0 // indirect

replace github.com/Yeah114/bdump => ../../third_party/fatalder/modules/WaterStructure/modules/bdump
