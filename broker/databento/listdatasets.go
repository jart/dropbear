package databento

// ListDatasets retrieves the list of available datasets from Databento.
func (c *HistoricalClient) ListDatasets() ([]string, error) {
	var datasets []string
	err := c.getMetadataJSON("/v0/metadata.list_datasets", nil, &datasets)
	if err != nil {
		return nil, err
	}
	return datasets, nil
}
