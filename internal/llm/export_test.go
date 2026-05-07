package llm

// ExportJSONSchemaFromValue はテスト専用のエクスポートラッパーです。
// jsonSchemaFromValue を package 外部から呼び出せるようにします。
var ExportJSONSchemaFromValue = jsonSchemaFromValue

// ExportJSONSchemaFromType はテスト専用のエクスポートラッパーです。
// jsonSchemaFromType を package 外部から呼び出せるようにします。
var ExportJSONSchemaFromType = jsonSchemaFromType

// ExportSanitizeJSONResponse はテスト専用のエクスポートラッパーです。
// sanitizeJSONResponse を package 外部から呼び出せるようにします。
var ExportSanitizeJSONResponse = sanitizeJSONResponse

// ExportExtractWaitSeconds はテスト専用のエクスポートラッパーです。
var ExportExtractWaitSeconds = extractWaitSeconds

// ExportIsDailyLimitMessage はテスト専用のエクスポートラッパーです。
var ExportIsDailyLimitMessage = isDailyLimitMessage
