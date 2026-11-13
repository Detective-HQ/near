import Link from "next/link";
import "./workspace/workspace.css";

export default function NotFound() {
  return (
    <main className="ws-fullpage">
      <h1>Page not found.</h1>
      <Link className="button" href="/workspace/explore">Go to Explore</Link>
    </main>
  );
}
