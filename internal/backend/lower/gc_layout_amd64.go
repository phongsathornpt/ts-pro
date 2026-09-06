package lower

// amd64GCTraceKind describes how a heap object exposes child references.
// The table is the single source of truth for GC tracing dispatch; runtime
// object type IDs remain stable because they are stored in native heap headers.
type amd64GCTraceKind uint8

const (
	amd64GCTraceAtomic amd64GCTraceKind = iota
	amd64GCTraceArray
	amd64GCTraceRefData
	amd64GCTraceObject
	amd64GCTraceClosure
	amd64GCTraceJSValueData
	amd64GCTraceDynamicObject
	amd64GCTraceDynamicEntries
	amd64GCTraceTask
	amd64GCTraceChannel
	amd64GCTraceTaskGroup
	amd64GCTraceCollection
)

type amd64GCLayoutDescriptor struct {
	ObjectType int64
	TraceKind  amd64GCTraceKind
}

var amd64GCLayouts = [...]amd64GCLayoutDescriptor{
	{amd64ObjectTypeAtomic, amd64GCTraceAtomic},
	{amd64ObjectTypeArray, amd64GCTraceArray},
	{amd64ObjectTypeArrayData, amd64GCTraceAtomic},
	{amd64ObjectTypeRefData, amd64GCTraceRefData},
	{amd64ObjectTypeObject, amd64GCTraceObject},
	{amd64ObjectTypeClosure, amd64GCTraceClosure},
	{amd64ObjectTypeJSValueData, amd64GCTraceJSValueData},
	{amd64ObjectTypeDynamicObject, amd64GCTraceDynamicObject},
	{amd64ObjectTypeDynamicEntries, amd64GCTraceDynamicEntries},
	{amd64ObjectTypeTask, amd64GCTraceTask},
	{amd64ObjectTypeChannel, amd64GCTraceChannel},
	{amd64ObjectTypeTaskGroup, amd64GCTraceTaskGroup},
	{amd64ObjectTypeCollection, amd64GCTraceCollection},
}

func amd64GCLayoutForType(objectType int64) (amd64GCLayoutDescriptor, bool) {
	if objectType < 0 || objectType >= int64(len(amd64GCLayouts)) {
		return amd64GCLayoutDescriptor{}, false
	}
	desc := amd64GCLayouts[objectType]
	return desc, desc.ObjectType == objectType
}
