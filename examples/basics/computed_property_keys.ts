interface User {
  id: number;
  name: string;
}

export function testComputed(): void {
  const u: User = { id: 101, name: "Alice" };
  console.log(u["id"]);
  console.log(u["name"]);

  const k = "name";
  const u2: any = u;
  console.log(u2[k]);
  u2["id"] = 202;
  console.log(u2["id"]);

  const headers = {
    "content-type": "application/json",
    accept: "text/html",
  };
  console.log(headers["content-type"]);
  console.log(headers["accept"]);

  const arr = [10, 20, 30];
  const idx: any = 1;
  console.log(arr[idx]);
  arr[idx] = 99;
  console.log(arr[idx]);
}
testComputed();
