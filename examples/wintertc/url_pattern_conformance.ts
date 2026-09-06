// WinterTC URLPattern Conformance & Differential Test

// 1. String construction
const p1 = new URLPattern("https://example.com:8080/books/:id\\?sort=:sort#section");
console.log(p1.protocol);
console.log(p1.hostname);
console.log(p1.port);
console.log(p1.pathname);
console.log(p1.search);
console.log(p1.hash);
console.log(p1.hasRegExpGroups);

// 2. Relative pattern with base
const p2 = new URLPattern("/api/:version/*", "https://example.org");
console.log(p2.protocol);
console.log(p2.hostname);
console.log(p2.pathname);

// 3. Init object
const p3 = new URLPattern({ pathname: "/users/:id" });
console.log(p3.protocol);
console.log(p3.hostname);
console.log(p3.pathname);

// 4. Init object with baseURL
const p4 = new URLPattern({ pathname: "/articles/:slug", baseURL: "https://example.net:3000" });
console.log(p4.protocol);
console.log(p4.hostname);
console.log(p4.port);
console.log(p4.pathname);

// 5. Error handling
try {
  new URLPattern("/relative-without-base");
  console.log("bad");
} catch (e: any) {
  console.log(e.name);
}

try {
  new URLPattern({ pathname: "/test" }, "https://example.com");
  console.log("bad");
} catch (e: any) {
  console.log(e.name);
}

// 6. Test method
console.log(p1.test("https://example.com:8080/books/42?sort=asc#section"));
console.log(p1.test("https://example.com:8080/books/42?sort=asc#other"));
console.log(p1.test("https://example.com:8080/authors/42"));
console.log(p2.test("https://example.org/api/v1/posts/100"));
console.log(p2.test("https://example.org/other/v1/posts"));
console.log(p2.test("/api/v2/items", "https://example.org"));
console.log(p3.test({ pathname: "/users/99" }));
console.log(p3.test({ pathname: "/products/99" }));
console.log(p4.test("https://example.net:3000/articles/hello-world"));
console.log(p4.test("https://example.net:4000/articles/hello-world"));

// 7. Exec method
const r1 = p1.exec("https://example.com:8080/books/42?sort=asc#section");
if (r1 !== null) {
  console.log(r1.pathname.input);
  const pGroups: any = r1.pathname.groups;
  console.log(pGroups.id);
  const sGroups: any = r1.search.groups;
  console.log(sGroups.sort);
  console.log(r1.inputs.length);
  console.log(r1.inputs[0]);
}

const r2 = p2.exec("https://example.org/api/v1/posts/100");
if (r2 !== null) {
  const pGroups: any = r2.pathname.groups;
  console.log(pGroups.version);
  console.log(pGroups["0"]);
}

const rNull = p1.exec("https://example.com:8080/nomatch");
console.log(rNull === null);

// 8. Regex groups
const pRegex = new URLPattern("https://example.com/items/:id(\\d+)");
console.log(pRegex.hasRegExpGroups);
console.log(pRegex.test("https://example.com/items/777"));
console.log(pRegex.test("https://example.com/items/abc"));
const rRegex = pRegex.exec("https://example.com/items/777");
if (rRegex !== null) {
  const pGroups: any = rRegex.pathname.groups;
  console.log(pGroups.id);
}

// 9. Regex alternation
const pAlt = new URLPattern("https://example.com/(api|v1)/data");
console.log(pAlt.hasRegExpGroups);
console.log(pAlt.test("https://example.com/api/data"));
console.log(pAlt.test("https://example.com/v1/data"));
console.log(pAlt.test("https://example.com/v2/data"));
const rAlt = pAlt.exec("https://example.com/api/data");
if (rAlt !== null) {
  const pGroups: any = rAlt.pathname.groups;
  console.log(pGroups["0"]);
}
