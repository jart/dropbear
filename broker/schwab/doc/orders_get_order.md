# Schwab Trader APIs

## Get Order

GET /accounts/{accountNumber}/orders/{orderId}

### Responses

- 200 will contain [order.schema.json](order.schema.json)

- 400 will have [error.schema.json](error.schema.json). An error message
  indicating the validation problem with the request.

- 401 will have [error.schema.json](error.schema.json). An error message
  indicating either authorization token is invalid or there are no
  accounts the caller is allowed to view or use for trading that are
  registered with the provided third party application.

- 403 will have [error.schema.json](error.schema.json). An error message
  indicating the caller is forbidden from accessing this service.

- 404 will have [error.schema.json](error.schema.json). An error message
  indicating the resource is not found.

- 500 will have [error.schema.json](error.schema.json). An error message
  indicating there was an unexpected server error.

- 503 will have [error.schema.json](error.schema.json). An error message
  indicating server has a temporary problem responding.
