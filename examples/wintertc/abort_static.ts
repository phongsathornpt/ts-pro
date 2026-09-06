const immediate = AbortSignal.abort("static");
console.log(immediate.aborted);
console.log(immediate.reason);
const defaulted = AbortSignal.abort();
try { defaulted.throwIfAborted(); } catch (reason: any) { console.log(reason.name); }

const timed = AbortSignal.timeout(0);
timed.addEventListener("abort", (event: Event): void => {
  console.log(timed.reason.name);
});

const first = new AbortController();
const second = new AbortController();
const dependent = AbortSignal.any([first.signal, second.signal]);
dependent.addEventListener("abort", (event: Event): void => {
  console.log(dependent.reason);
});
second.abort("second");
console.log(dependent.reason);

const already = AbortSignal.any([AbortSignal.abort("first"), AbortSignal.abort("later")]);
console.log(already.reason);
