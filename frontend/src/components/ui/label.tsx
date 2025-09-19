import React from "react";

type LabelProps = React.LabelHTMLAttributes<HTMLLabelElement>;

export const Label: React.FC<LabelProps> = ({
  children,
  className = "",
  ...props
}) => {
  return (
    <label
      {...props}
      className={`block text-sm font-medium text-foreground ${className}`}
    >
      {children}
    </label>
  );
};
