package wintertc_test

import (
	"testing"
)

func TestLinuxAMD64WinterTCQueuingStrategies(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_queuing_strategies",
		source: `
const bl = new ByteLengthQueuingStrategy({ highWaterMark: 512 });
console.log(bl.highWaterMark);
const u8 = new Uint8Array(8);
console.log(bl.size(u8));

const cs = new CountQueuingStrategy({ highWaterMark: 3 });
console.log(cs.highWaterMark);
console.log(cs.size("test"));
`,
		expected: "512\n8\n3\n1\n",
	})
}

func TestLinuxAMD64WinterTCReadableStreamCore(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_readable_stream_core",
		source: `
async function test(): Promise<void> {
  const rs = new ReadableStream({
    start: (controller: ReadableStreamDefaultController): void => {
      console.log(controller.desiredSize);
      controller.enqueue("a");
      controller.enqueue("b");
      controller.close();
    }
  });
  console.log(rs.locked);
  const reader = rs.getReader();
  console.log(rs.locked);

  const r1 = await reader.read();
  console.log(r1.value);
  console.log(r1.done);

  const r2 = await reader.read();
  console.log(r2.value);
  console.log(r2.done);

  const r3 = await reader.read();
  console.log(r3.done);

  reader.releaseLock();
  console.log(rs.locked);
}
test();
`,
		expected: "1\nfalse\ntrue\na\nfalse\nb\nfalse\ntrue\nfalse\n",
	})
}

func TestLinuxAMD64WinterTCWritableStreamCore(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_writable_stream_core",
		source: `
async function test(): Promise<void> {
  const items: string[] = [];
  const ws = new WritableStream({
    write: (chunk: any): void => {
      items.push(chunk);
    }
  });
  console.log(ws.locked);
  const writer = ws.getWriter();
  console.log(ws.locked);
  await writer.write("msg1");
  await writer.write("msg2");
  await writer.close();
  console.log(items.length);
  console.log(items[0]);
  console.log(items[1]);
}
test();
`,
		expected: "false\ntrue\n2\nmsg1\nmsg2\n",
	})
}

func TestLinuxAMD64WinterTCTransformStreamPipe(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_transform_stream_pipe",
		source: `
async function test(): Promise<void> {
  const ts = new TransformStream({
    transform: (chunk: any, controller: any): void => {
      controller.enqueue("got:" + chunk);
    }
  });
  const rs = ReadableStream.from(["one", "two"]);
  const piped = rs.pipeThrough(ts);
  const reader = piped.getReader();
  const c1 = await reader.read();
  console.log(c1.value);
  const c2 = await reader.read();
  console.log(c2.value);
  const c3 = await reader.read();
  console.log(c3.done);
}
test();
`,
		expected: "got:one\ngot:two\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCStreamsConformance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_streams_conformance",
		source:   mustReadExample(t, "../../examples/wintertc/streams_conformance.ts"),
		expected: "1024\n16\n10\n1\n1\nfalse\ntrue\nchunk1\nfalse\nchunk2\nfalse\ntrue\nfalse\nalpha\nbeta\ntrue\ntrue\nbranch-data\nbranch-data\nfalse\ntrue\n1\n2\nwrite-1\nwrite-2\ntransformed:in1\ntransformed:in2\ntrue\nutf-8\nutf-8\nfalse\nfalse\nblob-stream-content\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCBodyStreamBacking(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_body_stream_backing",
		source: `
function test(): void {
  const emptyResponse = new Response();
  console.log(emptyResponse.body === null);

  const response = new Response("response-body");
  console.log(response.body === null);
  console.log(response.body === response.body);

  const emptyRequest = new Request("https://example.com/");
  console.log(emptyRequest.body === null);

  const request = new Request("https://example.com/", { method: "POST", body: "request-body" });
  console.log(request.body === null);
  console.log(request.body === request.body);
}
test();
`,
		expected: "true\nfalse\ntrue\ntrue\nfalse\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCBodyStreamDisturbance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_body_stream_disturbance",
		source: `
async function test(): Promise<void> {
  const response = new Response("abc");
  console.log(response.bodyUsed);
  const responseStream = response.body ?? new ReadableStream();
  const responseReader = responseStream.getReader();
  const responseChunk: any = await responseReader.read();
  console.log(response.bodyUsed);
  const responseBytes: Uint8Array = responseChunk.value;
  console.log(responseBytes.length);
  console.log(responseBytes[0]);

  const request = new Request("https://example.com/", { method: "POST", body: "xyz" });
  console.log(request.bodyUsed);
  const requestStream = request.body ?? new ReadableStream();
  const requestReader = requestStream.getReader();
  await requestReader.cancel();
  console.log(request.bodyUsed);
}
test();
`,
		expected: "false\ntrue\n3\n97\nfalse\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCBodyLockedIsUnusable(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_body_locked_unusable",
		source: `
async function test(): Promise<void> {
  const response = new Response("abc");
  const responseStream = response.body ?? new ReadableStream();
  responseStream.getReader();
  console.log(response.bodyUsed);
  try {
    await response.text();
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
  try {
    response.clone();
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }

  const request = new Request("https://example.com/", { method: "POST", body: "xyz" });
  const requestStream = request.body ?? new ReadableStream();
  requestStream.getReader();
  console.log(request.bodyUsed);
  try {
    request.clone();
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
test();
`,
		expected: "false\nTypeError\nTypeError\nfalse\nTypeError\n",
	})
}
