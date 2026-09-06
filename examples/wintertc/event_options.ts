const target = new EventTarget();
const callback = (event: Event): void => { console.log("callback"); };
target.addEventListener("tick", callback, true);
target.addEventListener("tick", callback, true);
target.addEventListener("tick", callback, false);
target.dispatchEvent(new Event("tick"));
target.removeEventListener("tick", callback, true);
target.dispatchEvent(new Event("tick"));
target.removeEventListener("tick", callback, { capture: false });
target.dispatchEvent(new Event("tick"));

const passiveTarget = new EventTarget();
passiveTarget.addEventListener("cancel", (event: Event): void => {
  event.preventDefault();
}, { passive: true });
const passiveEvent = new Event("cancel", { cancelable: true });
console.log(passiveTarget.dispatchEvent(passiveEvent));
console.log(passiveEvent.defaultPrevented);

const activeTarget = new EventTarget();
activeTarget.addEventListener("cancel", (event: Event): void => {
  event.preventDefault();
}, { passive: false });
const activeEvent = new Event("cancel", { cancelable: true });
console.log(activeTarget.dispatchEvent(activeEvent));
console.log(activeEvent.defaultPrevented);
