// WinterTC Minimum Common Web API - Blob, File, FormData Conformance Surface
// ECMA-429 § 7.4 (WHATWG File API & Fetch FormData)

async function runBlobFileFormDataConformance(): Promise<void> {
  // 1. Blob constructor & properties
  const b1 = new Blob();
  console.log(b1.size);
  console.log(b1.type === "" ? "empty-type" : b1.type);

  const b2 = new Blob(["hello ", "world"], { type: "TEXT/PLAIN; charset=utf-8" });
  console.log(b2.size);
  console.log(b2.type);
  const text2 = await b2.text();
  console.log(text2);

  // 2. Blob slice
  const s1 = b2.slice(0, 5);
  console.log(s1.size);
  const textS1 = await s1.text();
  console.log(textS1);

  const s2 = b2.slice(-5);
  console.log(s2.size);
  const textS2 = await s2.text();
  console.log(textS2);

  const s3 = b2.slice(0, 5, "IMAGE/PNG");
  console.log(s3.type);

  // 3. Blob arrayBuffer & bytes
  const ab = await b2.arrayBuffer();
  console.log(ab.byteLength);

  const bytes = await b2.bytes();
  console.log(bytes.length);
  console.log(bytes[0]);
  console.log(bytes[10]);

  // 4. File constructor & properties
  const f1 = new File(["file-data"], "test.txt");
  console.log(f1.name);
  console.log(f1.size);
  console.log(f1.lastModified > 0);

  const f2 = new File(["content"], "custom.bin", { type: "APPLICATION/OCTET-STREAM", lastModified: 999999 });
  console.log(f2.name);
  console.log(f2.type);
  console.log(f2.lastModified);
  const textF2 = await f2.text();
  console.log(textF2);

  // 5. FormData core operations
  const fd = new FormData();
  fd.append("a", "1");
  fd.append("b", "2");
  fd.append("a", "3");
  console.log(fd.get("a"));
  console.log(fd.get("missing") === null ? "null-val" : "not-null");
  console.log(fd.has("b"));
  console.log(fd.has("c"));

  const allA = fd.getAll("a");
  console.log(allA.length);
  console.log(allA[0]);
  console.log(allA[1]);

  fd.set("a", "100");
  const newA = fd.getAll("a");
  console.log(newA.length);
  console.log(newA[0]);

  fd.delete("b");
  console.log(fd.has("b"));

  // 6. FormData with Blob & File coercion
  fd.append("fileBlob", new Blob(["blobby"]));
  const fb: File = fd.get("fileBlob");
  console.log(fb.name);
  const textFb = await fb.text();
  console.log(textFb);

  fd.append("namedBlob", new Blob(["named"]), "custom.dat");
  const nb: File = fd.get("namedBlob");
  console.log(nb.name);

  fd.append("fileObj", f2);
  const fo: File = fd.get("fileObj");
  console.log(fo.name);

  fd.append("overrideFile", f2, "renamed.bin");
  const overriddenFile: File = fd.get("overrideFile");
  console.log(overriddenFile.name);

  // 7. FormData forEach iteration
  const iterFd = new FormData();
  iterFd.append("x", "10");
  iterFd.append("y", "20");
  iterFd.append("x", "30");

  iterFd.forEach((val: any, key: string) => {
    console.log(key + ":" + val);
  });
}

runBlobFileFormDataConformance();
