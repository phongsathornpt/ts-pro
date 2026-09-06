const encoder = new TextEncoder();
console.log(encoder.encoding);
const bytes = encoder.encode("hé✓");
console.log(bytes.length);
for (let i = 0; i < bytes.length; i = i + 1) {
  console.log(bytes[i]);
}

const decoder = new TextDecoder("utf-8", { fatal: false, ignoreBOM: false });
console.log(decoder.encoding);
console.log(decoder.fatal);
console.log(decoder.ignoreBOM);
console.log(decoder.decode(bytes));
