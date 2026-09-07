const h = new Headers([
  ["X-B", "  two  "],
  ["x-a", "one"],
  ["X-A", "three"],
  ["set-cookie", "a=1"],
  ["Set-Cookie", "b=2"],
]);
console.log(h.get("X-A"));
console.log(h.has("x-b"));
console.log(h.get("missing"));
console.log(h.getSetCookie().length);
for (const x of h.getSetCookie()) console.log("cookie=" + x);
for (const pair of h.entries()) console.log(pair[0] + "=" + pair[1]);
h.set("x-a", " replacement ");
h.delete("x-b");
console.log("after=" + h.get("x-a"));
console.log("hasb=" + h.has("x-b"));
let each = "";
h.forEach((v: string, k: string): void => {
  each = each + "[" + k + ":" + v + "]";
});
console.log(each);
const copy = new Headers(h);
console.log("copy=" + copy.get("x-a"));
const rec = new Headers({ "X-Z": " z ", "x-y": "y" });
for (const pair of rec.entries()) console.log("rec=" + pair[0] + "=" + pair[1]);
try {
  new Headers().append("bad name", "x");
} catch (e: any) {
  console.log("name=" + e.name);
}
try {
  new Headers().append("ok", "bad\nvalue");
} catch (e: any) {
  console.log("value=" + e.name);
}
