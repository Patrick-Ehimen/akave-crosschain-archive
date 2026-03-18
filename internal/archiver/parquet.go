package archiver

import (
	"bytes"
	"fmt"
	"io"

	"github.com/parquet-go/parquet-go"
)

// WriteParquet serializes records to Parquet format with Snappy compression.
// Returns a buffer containing the Parquet file bytes.
func WriteParquet(records []ParquetRecord) (*bytes.Buffer, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("no records to write")
	}

	buf := new(bytes.Buffer)

	writer := parquet.NewGenericWriter[ParquetRecord](buf,
		parquet.Compression(&parquet.Snappy),
	)

	_, err := writer.Write(records)
	if err != nil {
		return nil, fmt.Errorf("writing parquet records: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing parquet writer: %w", err)
	}

	return buf, nil
}

// ReadParquet deserializes a Parquet file from a reader into records.
// Used primarily in tests to verify round-trip correctness.
func ReadParquet(r io.ReaderAt, size int64) ([]ParquetRecord, error) {
	reader := parquet.NewGenericReader[ParquetRecord](r)
	defer reader.Close()

	records := make([]ParquetRecord, reader.NumRows())
	n, err := reader.Read(records)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("reading parquet records: %w", err)
	}

	return records[:n], nil
}
