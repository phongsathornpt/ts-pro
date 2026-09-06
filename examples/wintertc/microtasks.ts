console.log("sync");
queueMicrotask((): void => { console.log("micro-1"); });
queueMicrotask((): void => { console.log("micro-2"); });
spawn((): void => { console.log("task"); });
