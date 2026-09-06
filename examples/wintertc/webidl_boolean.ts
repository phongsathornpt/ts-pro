console.log(new Event("one", { cancelable: 1 }).cancelable);
console.log(new Event("zero", { cancelable: 0 }).cancelable);
console.log(new Event("text", { cancelable: "yes" }).cancelable);
console.log(new Event("empty", { cancelable: "" }).cancelable);
console.log(new Event("object", { cancelable: { value: 1 } }).cancelable);

const target = new EventTarget();
const passive = new Event("passive", { cancelable: 1 });
target.addEventListener("passive", (event: Event): void => {
  event.preventDefault();
}, { passive: "yes" });
console.log(target.dispatchEvent(passive));
console.log(passive.defaultPrevented);
