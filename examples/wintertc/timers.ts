console.log("sync");
const cancelled = setTimeout((): void => { console.log("cancelled"); }, 0);
clearTimeout(cancelled);
setTimeout((): void => { console.log("timer"); }, 0);
queueMicrotask((): void => { console.log("micro"); });
