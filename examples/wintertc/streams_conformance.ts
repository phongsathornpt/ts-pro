// WinterTC Minimum Common Web API - Streams API Conformance Surface
// ECMA-429 (WHATWG Streams Standard baseline)

async function runStreamsConformance(): Promise<void> {
  // 1. Queuing Strategies
  const bl = new ByteLengthQueuingStrategy({ highWaterMark: 1024 });
  console.log(bl.highWaterMark);
  const u8 = new Uint8Array(16);
  console.log(bl.size(u8));

  const cs = new CountQueuingStrategy({ highWaterMark: 10 });
  console.log(cs.highWaterMark);
  console.log(cs.size("anything"));

  // 2. ReadableStream basic lifecycle: start, enqueue, close, read
  const rs1 = new ReadableStream({
    start: (controller: any): void => {
      console.log(controller.desiredSize);
      controller.enqueue("chunk1");
      controller.enqueue("chunk2");
      controller.close();
    }
  });
  console.log(rs1.locked);
  const r1 = rs1.getReader();
  console.log(rs1.locked);

  const res1 = await r1.read();
  console.log(res1.value);
  console.log(res1.done);

  const res2 = await r1.read();
  console.log(res2.value);
  console.log(res2.done);

  const res3 = await r1.read();
  console.log(res3.done);

  r1.releaseLock();
  console.log(rs1.locked);

  // 3. ReadableStream.from static constructor
  const rsFrom = ReadableStream.from(["alpha", "beta"]);
  const rFrom = rsFrom.getReader();
  const f1 = await rFrom.read();
  console.log(f1.value);
  const f2 = await rFrom.read();
  console.log(f2.value);
  const f3 = await rFrom.read();
  console.log(f3.done);

  // 4. ReadableStream tee
  const rsToTee = ReadableStream.from(["branch-data"]);
  const branches = rsToTee.tee();
  const b1 = branches[0];
  const b2 = branches[1];
  console.log(rsToTee.locked);
  const rb1 = b1.getReader();
  const rb2 = b2.getReader();
  const rb1Val = await rb1.read();
  const rb2Val = await rb2.read();
  console.log(rb1Val.value);
  console.log(rb2Val.value);

  // 5. WritableStream lifecycle: start, write, close
  const written: string[] = [];
  const ws = new WritableStream({
    write: (chunk: any): void => {
      written.push(chunk);
    }
  });
  console.log(ws.locked);
  const writer = ws.getWriter();
  console.log(ws.locked);
  console.log(writer.desiredSize);
  await writer.write("write-1");
  await writer.write("write-2");
  await writer.close();
  console.log(written.length);
  console.log(written[0]);
  console.log(written[1]);

  // 6. TransformStream & pipeThrough
  const ts = new TransformStream({
    transform: (chunk: any, controller: any): void => {
      controller.enqueue("transformed:" + chunk);
    }
  });
  const sourceStream = ReadableStream.from(["in1", "in2"]);
  const pipedStream = sourceStream.pipeThrough(ts);
  const pipedReader = pipedStream.getReader();
  const p1 = await pipedReader.read();
  console.log(p1.value);
  const p2 = await pipedReader.read();
  console.log(p2.value);
  const p3 = await pipedReader.read();
  console.log(p3.done);

  // 7. TextEncoderStream & TextDecoderStream
  const encStream = new TextEncoderStream();
  console.log(encStream.encoding);
  const decStream = new TextDecoderStream();
  console.log(decStream.encoding);
  console.log(decStream.fatal);
  console.log(decStream.ignoreBOM);

  // 8. Blob.prototype.stream()
  const blob = new Blob(["blob-stream-content"]);
  const blobStream = blob.stream();
  const blobReader = blobStream.getReader();
  const blobChunk = await blobReader.read();
  const decodedBlobText = new TextDecoder().decode(blobChunk.value);
  console.log(decodedBlobText);
  const blobEnd = await blobReader.read();
  console.log(blobEnd.done);
}

runStreamsConformance();
