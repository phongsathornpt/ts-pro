const defaultError = new DOMException();
console.log(defaultError.name);
console.log(defaultError.message);

const explicit = new DOMException("bad byte", "InvalidCharacterError");
console.log(explicit.name);
console.log(explicit.message);
