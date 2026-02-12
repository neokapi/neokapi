import { InputGlass } from "shadcn-glass-ui";

// Re-export with proper React.InputHTMLAttributes typing (glass library lacks .d.ts files)
const Input = InputGlass as React.ForwardRefExoticComponent<
  React.InputHTMLAttributes<HTMLInputElement> & React.RefAttributes<HTMLInputElement>
>;

export { Input };
