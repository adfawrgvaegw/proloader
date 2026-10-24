package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	sjson "github.com/Yeah114/WaterStructure/utils/json"
)

type benchFunc func([]byte) (int, error)

func main() {
	var (
		filePath string
		warmup   int
		runs     int
	)

	flag.StringVar(&filePath, "file", "structure/bdx_runtimeIds_117.json", "path to the JSON file to benchmark")
	flag.IntVar(&warmup, "warmup", 1, "number of warm-up runs before timing")
	flag.IntVar(&runs, "runs", 5, "number of timed runs per implementation")
	flag.Parse()

	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read %s: %v\n", filePath, err)
		os.Exit(1)
	}

	fmt.Printf("Benchmark file: %s (%d bytes)\n", filePath, len(data))
	fmt.Printf("Warm-up runs: %d | Timed runs: %d\n", warmup, runs)

	sjsonCount, err := runBenchmark("utils/json", warmup, runs, data, benchmarkSJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "utils/json benchmark error: %v\n", err)
		os.Exit(1)
	}
	stdCount, err := runBenchmark("encoding/json", warmup, runs, data, benchmarkStdJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encoding/json benchmark error: %v\n", err)
		os.Exit(1)
	}

	if sjsonCount != stdCount {
		fmt.Fprintf(os.Stderr, "warning: result counts differ (utils/json=%d, encoding/json=%d)\n", sjsonCount, stdCount)
	}
}

func runBenchmark(name string, warmup, runs int, payload []byte, fn benchFunc) (int, error) {
	for i := 0; i < warmup; i++ {
		if _, err := fn(payload); err != nil {
			return 0, fmt.Errorf("%s warm-up %d failed: %w", name, i+1, err)
		}
	}

	fmt.Printf("\n[%s]\n", name)
	var total time.Duration
	var lastCount int
	for i := 0; i < runs; i++ {
		start := time.Now()
		count, err := fn(payload)
		if err != nil {
			return 0, fmt.Errorf("%s run %d failed: %w", name, i+1, err)
		}
		elapsed := time.Since(start)
		fmt.Printf("Run %d: %v (%d top-level entries)\n", i+1, elapsed, count)
		total += elapsed
		lastCount = count
	}
	if runs > 0 {
		avg := total / time.Duration(runs)
		fmt.Printf("Average: %v\n", avg)
	}
	return lastCount, nil
}

func benchmarkSJSON(data []byte) (int, error) {
	reader := sjson.NewJSONReader(bytes.NewReader(data))
	typ, err := reader.PeekType()
	if err != nil {
		return 0, err
	}
	switch typ {
	case sjson.JSONArray:
		return consumeArray(reader)
	case sjson.JSONObject:
		return consumeObject(reader)
	case sjson.JSONInvalid:
		return 0, fmt.Errorf("invalid JSON input")
	default:
		if _, _, err := reader.ReadValue(); err != nil {
			return 0, err
		}
		return 1, nil
	}
}

func consumeArray(reader *sjson.JSONReader) (int, error) {
	if err := reader.BeginArray(); err != nil {
		return 0, err
	}
	count := 0
	for {
		hasNext, err := reader.HasNextArrayValue()
		if err != nil {
			return 0, err
		}
		if !hasNext {
			break
		}
		if err := reader.SkipValue(); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func consumeObject(reader *sjson.JSONReader) (int, error) {
	if err := reader.BeginObject(); err != nil {
		return 0, err
	}
	count := 0
	for {
		_, done, err := reader.NextObjectKey()
		if err != nil {
			return 0, err
		}
		if done {
			break
		}
		if err := reader.SkipValue(); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func benchmarkStdJSON(data []byte) (int, error) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return 0, err
	}
	return countTopLevelEntries(v), nil
}

func countTopLevelEntries(v interface{}) int {
	switch val := v.(type) {
	case []interface{}:
		return len(val)
	case map[string]interface{}:
		return len(val)
	case nil:
		return 0
	default:
		return 1
	}
}
