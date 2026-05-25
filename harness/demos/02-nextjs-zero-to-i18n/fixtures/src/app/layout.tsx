export const metadata = {
  title: "Lumen Notes",
  description: "Your ideas, in sync.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
