const target = new EventTarget();
const controller = new AbortController();
let calls = 0;
const callback = (event: Event): void => {
  calls = calls + 1;
  console.log("signal-listener");
};
target.addEventListener("tick", callback, { signal: controller.signal });
target.dispatchEvent(new Event("tick"));
controller.abort();
target.dispatchEvent(new Event("tick"));
console.log(calls);

const already = new AbortController();
already.abort();
target.addEventListener("late", (event: Event): void => {
  console.log("should-not-run");
}, { signal: already.signal });
target.dispatchEvent(new Event("late"));
console.log("done");
