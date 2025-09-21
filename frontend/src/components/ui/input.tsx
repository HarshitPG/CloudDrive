import React from "react";

type InputProps = React.InputHTMLAttributes<HTMLInputElement>;

export const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className = "", ...props }, ref) => {
    return (
      <input
        {...props}
        ref={ref}
        className={`drive-surface w-full py-2 ${className}`}
      />
    );
  }
);

Input.displayName = "Input";
