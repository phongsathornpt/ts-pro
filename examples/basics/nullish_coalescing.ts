type User = {
  name: string;
  age?: number;
  profile?: { bio: string };
};

export function getDisplayName(name: string | null): string {
  return name ?? "Anonymous";
}

export function getCount(count: number | null): number {
  return count ?? 42;
}

export function getFlag(flag: boolean | null): boolean {
  return flag ?? true;
}

export function getUserBio(user: User | null): string {
  return user?.profile?.bio ?? "No bio available";
}

console.log(getDisplayName(null));
console.log(getDisplayName("Alice"));
console.log(getCount(null));
console.log(getCount(0));
console.log(getCount(100));
console.log(getFlag(null));
console.log(getFlag(false));
console.log(getFlag(true));

const userWithBio: User = { name: "Bob", profile: { bio: "Hello world!" } };
const userWithoutBio: User = { name: "Charlie" };
const nullUser: User | null = null;

console.log(getUserBio(userWithBio));
console.log(getUserBio(userWithoutBio));
console.log(getUserBio(nullUser));
