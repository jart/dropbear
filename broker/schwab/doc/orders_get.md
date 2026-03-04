# Schwab Trader APIs

## Get Orders

GET /accounts/{accountNumber}/orders

### Query Parameters

- `maxResults` The max number of orders to retrieve. Default is 3000.

- `fromEnteredTime` (required) Specifies that no orders entered before
  this time should be returned. Valid ISO-8601 formats are :
  yyyy-MM-dd'T'HH:mm:ss.SSSZ Example fromEnteredTime is
  '2024-03-29T00:00:00.000Z'. 'toEnteredTime' must also be set.

- `toEnteredTime` (required) Specifies that no orders entered after this
  time should be returned.Valid ISO-8601 formats are :
  yyyy-MM-dd'T'HH:mm:ss.SSSZ. Example toEnteredTime is
  '2024-04-28T23:59:59.000Z'. 'fromEnteredTime' must also be set.

- `status` may be AWAITING_PARENT_ORDER, AWAITING_CONDITION,
  AWAITING_STOP_CONDITION, AWAITING_MANUAL_REVIEW, ACCEPTED,
  AWAITING_UR_OUT, PENDING_ACTIVATION, QUEUED, WORKING, REJECTED,
  PENDING_CANCEL, CANCELED, PENDING_REPLACE, REPLACED, FILLED, EXPIRED,
  NEW, AWAITING_RELEASE_TIME, PENDING_ACKNOWLEDGEMENT, PENDING_RECALL,
  UNKNOWN

### Responses

- 200 will have an array of [order.schema.json](order.schema.json)

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
