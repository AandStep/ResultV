import React from "react";
import { Activity, ShoppingCart, Plus, List, Settings } from "lucide-react";
import { useConfigContext } from "../../context/ConfigContext";
import { Sidebar } from "./Sidebar";
import { MobileHeader } from "./MobileHeader";

const MobileNavItem = ({ icon, label, isActive, onClick }) => (
  <button
    onClick={onClick}
    className={`flex flex-col items-center p-2 min-w-[64px] border-transparent outline-none focus:outline-none focus:ring-0 focus-visible:outline-none ${
      isActive ? "text-[#007E3A]" : "text-zinc-500 hover:text-[#00A819]"
    }`}
  >
    {React.cloneElement(icon, { className: "w-6 h-6 mb-1" })}
    <span className="text-[10px] font-medium">{label}</span>
  </button>
);

export const MainLayout = ({ children }) => {
  const { activeTab, setActiveTab, setEditingProxy } = useConfigContext();

  return (
    <div className="fixed inset-0 flex bg-zinc-950 text-zinc-200 font-sans overflow-hidden select-none">
      <style>{`
        * { 
          outline: none !important; 
          -webkit-tap-highlight-color: transparent !important; 
        }
        button { border-color: transparent; }
        button:hover, a:hover { 
          border-color: transparent;
        }
        button:focus, input:focus, a:focus { 
          outline: none !important; 
          box-shadow: none !important; 
        }
        :root { --bs-primary: transparent; }
        .scrollbar-hide::-webkit-scrollbar { display: none; }
      `}</style>

      <Sidebar />

      <div className="flex-1 flex flex-col relative overflow-y-auto min-w-0">
        <MobileHeader />

        <div className="p-4 md:px-10 md:py-6 w-full max-w-[1600px] mx-auto pb-24 md:pb-8">
          {children}
        </div>
      </div>

      <div className="md:hidden absolute bottom-0 w-full bg-zinc-900 border-t border-zinc-800 flex justify-around p-2 z-20 pb-safe">
        <MobileNavItem
          icon={<Activity />}
          label="Главная"
          isActive={activeTab === "home"}
          onClick={() => setActiveTab("home")}
        />
        <MobileNavItem
          icon={<ShoppingCart />}
          label="Купить"
          isActive={activeTab === "buy"}
          onClick={() => setActiveTab("buy")}
        />
        <MobileNavItem
          icon={<Plus />}
          label="Добавить"
          isActive={activeTab === "add"}
          onClick={() => {
            setEditingProxy(null);
            setActiveTab("add");
          }}
        />
        <MobileNavItem
          icon={<List />}
          label="Прокси"
          isActive={activeTab === "list"}
          onClick={() => setActiveTab("list")}
        />
        <MobileNavItem
          icon={<Settings />}
          label="Настройки"
          isActive={activeTab === "settings"}
          onClick={() => setActiveTab("settings")}
        />
      </div>
    </div>
  );
};
