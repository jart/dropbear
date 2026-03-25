package databento

// ListSchemas retrieves the available schemas for a dataset from Databento.
func (c *HistoricalClient) ListSchemas(dataset string) ([]Schema, error) {
	var names []string
	err := c.getMetadataJSON("/v0/metadata.list_schemas",
		map[string]string{"dataset": dataset}, &names)
	if err != nil {
		return nil, err
	}
	schemas := make([]Schema, len(names))
	for i, name := range names {
		s, err := ParseSchema(name)
		if err != nil {
			return nil, err
		}
		schemas[i] = s
	}
	return schemas, nil
}
