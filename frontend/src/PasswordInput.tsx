import { useState } from "react";

const eye = <><path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0" /><circle cx="12" cy="12" r="3" /></>;
const eyeOff = <><path d="M10.733 5.076a10.744 10.744 0 0 1 11.205 6.575 1 1 0 0 1 0 .696 10.747 10.747 0 0 1-1.444 2.49" /><path d="M14.084 14.158a3 3 0 0 1-4.242-4.242" /><path d="M17.479 17.499a10.75 10.75 0 0 1-15.417-5.151 1 1 0 0 1 0-.696 10.75 10.75 0 0 1 4.446-5.143" /><path d="m2 2 20 20" /></>;

// PasswordInput is a password field with an eye that shows it; it takes the input's own props.
export default function PasswordInput({ className, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  const [show, setShow] = useState(false);
  return (
    <span className={`pw ${className ?? ""}`}>
      <input {...props} type={show ? "text" : "password"} />
      <button type="button" tabIndex={-1} title={show ? "隐藏" : "显示"} onClick={() => setShow(!show)}>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">{show ? eyeOff : eye}</svg>
      </button>
    </span>
  );
}
