package databento

// PublisherDetail describes a data publisher on Databento.
type PublisherDetail struct {
	PublisherID uint16 `json:"publisher_id"`
	Dataset     string `json:"dataset"`
	Venue       string `json:"venue"`
	Description string `json:"description"`
}

// ListPublishers retrieves the list of publishers from Databento.
func (c *HistoricalClient) ListPublishers() ([]PublisherDetail, error) {
	var publishers []PublisherDetail
	err := c.getMetadataJSON("/v0/metadata.list_publishers", nil, &publishers)
	if err != nil {
		return nil, err
	}
	return publishers, nil
}
