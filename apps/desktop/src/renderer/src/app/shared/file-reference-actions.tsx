import { createContext, type ReactNode, useContext } from "react";

export type FileReferenceTarget = {
  endLine?: number;
  path: string;
  raw: string;
  startLine?: number;
};

export type FileReferenceActions = {
  openFileReference: (target: FileReferenceTarget) => void;
};

const FileReferenceActionsContext = createContext<FileReferenceActions | null>(
  null,
);

export function FileReferenceActionsProvider({
  children,
  value,
}: {
  children: ReactNode;
  value: FileReferenceActions | null;
}) {
  return (
    <FileReferenceActionsContext.Provider value={value}>
      {children}
    </FileReferenceActionsContext.Provider>
  );
}

export function useFileReferenceActions() {
  return useContext(FileReferenceActionsContext);
}
