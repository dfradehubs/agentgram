"use client";

import { useUser } from "@/hooks/useUser";
import { usePreferencesContext } from "@/contexts/PreferencesContext";
import { useT } from "@/lib/i18n";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { AgentgramLogo } from "@/components/icons/AgentgramLogo";
import {
  Github,
  Globe,
  LogOut,
  Monitor,
  Moon,
  PanelLeft,
  PanelLeftClose,
  Settings,
  Shield,
  Sun,
} from "lucide-react";
import Link from "next/link";

interface HeaderProps {
  onToggleSidebar?: () => void;
  sidebarOpen?: boolean;
}

export function Header({ onToggleSidebar, sidebarOpen }: HeaderProps) {
  const { user, isAdmin, displayName, logout, disconnectGitHub } = useUser();
  const { preferences, updatePreference } = usePreferencesContext();
  const t = useT();

  const initial = displayName.charAt(0).toUpperCase();

  const themeIcon =
    preferences.theme === "dark" ? (
      <Moon className="h-3.5 w-3.5" />
    ) : preferences.theme === "light" ? (
      <Sun className="h-3.5 w-3.5" />
    ) : (
      <Monitor className="h-3.5 w-3.5" />
    );

  return (
    <header className="flex h-12 items-center justify-between border-b bg-background px-4">
      <div className="flex items-center gap-2">
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          onClick={onToggleSidebar}
          aria-label={sidebarOpen ? t("sidebar.close") : t("sidebar.open")}
        >
          {sidebarOpen ? (
            <PanelLeftClose className="h-4 w-4" />
          ) : (
            <PanelLeft className="h-4 w-4" />
          )}
        </Button>
        <Separator orientation="vertical" className="h-4" />
        <Link href="/" className="flex items-center gap-2 hover:opacity-80 transition-opacity">
          <AgentgramLogo className="h-5 w-5 text-primary" />
          <span className="text-sm font-semibold">Agentgram</span>
        </Link>
      </div>
      <div className="flex items-center gap-2">
        <span className="hidden text-xs text-muted-foreground sm:inline">
          {user?.email && user.email !== "anonymous@localhost"
            ? user.email
            : displayName}
        </span>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="h-8 w-8" aria-label={t("settings.label")}>
              <Settings className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            <DropdownMenuLabel className="flex items-center gap-2">
              {themeIcon}
              {t("settings.theme")}
            </DropdownMenuLabel>
            <DropdownMenuRadioGroup
              value={preferences.theme}
              onValueChange={(v) =>
                updatePreference("theme", v as "light" | "dark" | "system")
              }
            >
              <DropdownMenuRadioItem value="system">
                {t("settings.themeSystem")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="light">
                {t("settings.themeLight")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="dark">
                {t("settings.themeDark")}
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>

            <DropdownMenuSeparator />

            <DropdownMenuLabel>{t("settings.chatWidth")}</DropdownMenuLabel>
            <DropdownMenuRadioGroup
              value={preferences.chatWidth}
              onValueChange={(v) =>
                updatePreference(
                  "chatWidth",
                  v as "normal" | "wide" | "full"
                )
              }
            >
              <DropdownMenuRadioItem value="normal">
                {t("settings.chatWidthNormal")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="wide">
                {t("settings.chatWidthWide")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="full">
                {t("settings.chatWidthFull")}
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>

            <DropdownMenuSeparator />

            <DropdownMenuLabel className="flex items-center gap-2">
              <Globe className="h-3.5 w-3.5" />
              {t("settings.language")}
            </DropdownMenuLabel>
            <DropdownMenuRadioGroup
              value={preferences.locale}
              onValueChange={(v) =>
                updatePreference("locale", v as "es" | "en")
              }
            >
              <DropdownMenuRadioItem value="es">
                Espa&#241;ol
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="en">
                English
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>

            <DropdownMenuSeparator />

            <DropdownMenuLabel className="flex items-center gap-2">
              <Github className="h-3.5 w-3.5" />
              GitHub
            </DropdownMenuLabel>
            {user?.githubConnected ? (
              <DropdownMenuItem onClick={disconnectGitHub}>
                {t("settings.disconnectGithub", { name: user.githubUsername || "conectado" })}
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem onClick={() => window.location.href = "/auth/github/login"}>
                {t("settings.connectGithub")}
              </DropdownMenuItem>
            )}

            {isAdmin && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild>
                  <Link href="/admin" className="flex items-center gap-2">
                    <Shield className="h-3.5 w-3.5" />
                    Admin
                  </Link>
                </DropdownMenuItem>
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>

        <Avatar className="h-6 w-6">
          <AvatarFallback className="text-[10px]">{initial}</AvatarFallback>
        </Avatar>
        {user && user.email !== "anonymous@localhost" && (
          <Button variant="ghost" size="icon" onClick={logout} className="h-8 w-8" aria-label={t("settings.logout")}>
            <LogOut className="h-3.5 w-3.5" />
          </Button>
        )}
      </div>
    </header>
  );
}
