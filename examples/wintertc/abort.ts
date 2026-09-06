const controller = new AbortController();
console.log(controller.signal.aborted);
controller.signal.addEventListener("abort", (event: Event): void => {
  console.log(event.type);
});
controller.abort("stop");
console.log(controller.signal.aborted);
for (let i = 0; i < 50000; i = i + 1) {
  const dead = "garbage-" + i;
}
console.log(controller.signal.reason);
controller.abort("ignored");
console.log(controller.signal.reason);
try {
  controller.signal.throwIfAborted();
} catch (reason: any) {
  console.log(reason);
}
const defaults = new AbortController();
defaults.abort();
try {
  defaults.signal.throwIfAborted();
} catch (reason: any) {
  console.log(reason.name);
}
const handlers = new AbortController();
const oldHandler = (event: Event): void => { console.log("old-handler"); };
handlers.signal.onabort = oldHandler;
handlers.signal.onabort = (event: Event): void => { console.log("onabort"); };
handlers.abort();
const cleared = new AbortController();
cleared.signal.onabort = (event: Event): void => { console.log("should-not-run"); };
cleared.signal.onabort = null;
cleared.abort();
