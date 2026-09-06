const origin = performance.timeOrigin;
const start = performance.now();
const end = performance.now();
const json = performance.toJSON();
console.log(origin > 1600000000000);
console.log(start >= 0);
console.log(start < 60000);
console.log(end >= start);
console.log(json.timeOrigin === origin);
