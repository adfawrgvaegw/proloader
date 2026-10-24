module proloader.local/mpay-login

go 1.23.0

require github.com/Yeah114/g79client v0.0.0

require (
	github.com/database64128/chacha8-go v0.0.0-20250815115417-e0f2726d8bd0 // indirect
	github.com/google/uuid v1.6.0 // indirect
)

replace github.com/Yeah114/g79client => ../../third_party/fatalder/modules/g79client
