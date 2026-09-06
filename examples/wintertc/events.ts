const target = new EventTarget();
const first = (event: Event): void => {
  console.log("first");
};
const once = (event: Event): void => {
  console.log(event.type);
  event.preventDefault();
};

target.addEventListener("tick", first);
target.addEventListener("tick", first);
target.addEventListener("tick", once, { once: true });
console.log(target.dispatchEvent(new Event("tick", { cancelable: true })));
console.log(target.dispatchEvent(new Event("tick", { cancelable: true })));
target.removeEventListener("tick", first);
console.log(target.dispatchEvent(new Event("tick", { cancelable: true })));

const stopped = new EventTarget();
const stopper = (event: Event): void => {
  console.log("stop");
  event.stopImmediatePropagation();
};
const skipped = (event: Event): void => console.log("skipped");
stopped.addEventListener("go", stopper);
stopped.addEventListener("go", skipped);
console.log(stopped.dispatchEvent(new Event("go")));
