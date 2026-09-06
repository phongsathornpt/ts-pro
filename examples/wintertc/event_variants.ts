const custom = new CustomEvent("custom", { detail: "payload" });
console.log(custom.type);
console.log(custom.detail);

const message = new MessageEvent("message", {
  data: "hello",
  origin: "https://example.test",
  lastEventId: "42",
});
console.log(message.data);
console.log(message.origin);
console.log(message.lastEventId);

const error = new ErrorEvent("error", {
  message: "boom",
  filename: "app.ts",
  lineno: 12,
  colno: 7,
  error: "reason",
});
console.log(error.message);
console.log(error.filename);
console.log(error.lineno);
console.log(error.colno);
console.log(error.error);
