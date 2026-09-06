const fatal = new TextDecoder("utf-8", { fatal: true });
console.log(fatal.fatal);
console.log(fatal.ignoreBOM);
const bom = new TextDecoder("utf-8", { ignoreBOM: true });
console.log(bom.fatal);
console.log(bom.ignoreBOM);
