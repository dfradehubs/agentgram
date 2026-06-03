import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
import { AgentProvider } from "@/contexts/AgentContext";
import { SessionProvider } from "@/contexts/SessionContext";
import { UserProvider } from "@/contexts/UserContext";
import { PreferencesProvider } from "@/contexts/PreferencesContext";
import { ConfigProvider } from "@/contexts/ConfigContext";
import { MCPProvider } from "@/contexts/MCPContext";
import { BackgroundStreamProvider } from "@/contexts/BackgroundStreamContext";
import { ReadStateProvider } from "@/contexts/ReadStateContext";
import { Toaster } from "sonner";
import { TooltipProvider } from "@/components/ui/tooltip";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "Agentgram",
  description: "Unified interface for multiple AI agents",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="es" suppressHydrationWarning>
      <head>
        <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover" />
        <link rel="icon" href="/agentgram-logo.svg" type="image/svg+xml" />
        {/* SECURITY: Safe — static string literal, no user input. Do not add dynamic data here. */}
        <script
          dangerouslySetInnerHTML={{
            __html: `(function(){try{var s=localStorage.getItem('agentgram-preferences');var p=s?JSON.parse(s):{};var t=p.theme||'system';if(t==='dark'||(t==='system'&&matchMedia('(prefers-color-scheme:dark)').matches))document.documentElement.classList.add('dark');document.documentElement.lang=p.locale||'es'}catch(e){if(matchMedia('(prefers-color-scheme:dark)').matches)document.documentElement.classList.add('dark')}})()`,
          }}
        />
      </head>
      <body
        className={`${geistSans.variable} ${geistMono.variable} antialiased`}
      >
        <PreferencesProvider>
          <ConfigProvider>
            <UserProvider>
              <AgentProvider>
                <MCPProvider>
                  <SessionProvider>
                    <ReadStateProvider>
                      <BackgroundStreamProvider>
                        <TooltipProvider>
                          {children}
                        </TooltipProvider>
                      </BackgroundStreamProvider>
                    </ReadStateProvider>
                  </SessionProvider>
                  <Toaster position="bottom-right" richColors closeButton />
                </MCPProvider>
              </AgentProvider>
            </UserProvider>
          </ConfigProvider>
        </PreferencesProvider>
      </body>
    </html>
  );
}
