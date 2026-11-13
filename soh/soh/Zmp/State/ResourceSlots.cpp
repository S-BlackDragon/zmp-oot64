#include "ResourceSlots.h"
#include "FixedHeap.h"

#include <algorithm>
#include <atomic>
#include <cstdlib>
#include <mutex>
#include <new>
#include <string>
#include <unordered_map>
#include <vector>

#ifdef _WIN32
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <malloc.h>
#endif
#ifdef __EMSCRIPTEN__
#include <chrono>
#include <cstring>
#include <zip.h>
#endif

#include <ship/Context.h>
#include <ship/resource/File.h>
#include <ship/resource/Resource.h>
#include <ship/resource/ResourceFactory.h>
#include <ship/resource/ResourceManager.h>
#include <ship/resource/archive/ArchiveManager.h>

#include "soh/Zmp/ZmpLog.h"

namespace Zmp::ResSlots {

namespace {

constexpr uint64_t kSlotSize = 64ull << 20; // virtual space reserved per resource (Windows)
constexpr uint32_t kMaxSlots = 1u << 16;    // 65536 files -> 4 TiB of address space
constexpr uint64_t kPage = 4096;
#ifdef __EMSCRIPTEN__
// ZMP web (D-243): one linear memory and no virtual reservations, so the slots are carved out of a static region of
// the module (BSS: the same address in every browser that runs this wasm). Each file's slot is sized from its
// uncompressed size and its resource type, read from the archives' central directories and headers when the index
// is built; the factors come from measuring the desktop build (a display list parses into ~9x its file plus ~5 KiB
// of fixed cost, a texture into ~1x). Total for the reference assets: ~490 MiB on 64-bit, less here.
constexpr uint64_t kWasmRegionSize = 512ull << 20;
alignas(64) uint8_t sWasmRegion[kWasmRegionSize];
std::vector<uint64_t> sWasmOffsets; // per index: offset of the slot in the region
std::vector<uint64_t> sWasmSizes;   // per index: its size
inline uint64_t RegionBaseAddr() {
    return (uint64_t)(uintptr_t)sWasmRegion;
}
inline uint64_t RegionEndAddr() {
    return RegionBaseAddr() + kWasmRegionSize;
}
#else
inline uint64_t RegionBaseAddr() {
    return ZMP_RESOURCE_REGION_ADDR;
}
inline uint64_t RegionEndAddr() {
    return ZMP_RESOURCE_REGION_ADDR + kSlotSize * kMaxSlots;
}
#endif

struct Slot {
    uint64_t base = 0;
    uint64_t size = kSlotSize; // (Windows: the reserved range; web: this file's share of the region)
    uint64_t used = 0;
    uint64_t committed = 0;
    uint32_t index = 0;
    uint32_t loads = 0;
    std::atomic<bool> busy{ false };
    std::string path;
};

thread_local Slot* tSlot = nullptr;
thread_local int tPaused = 0;

std::mutex sMutex;
bool sIndexBuilt = false;
bool sActive = false;
std::unordered_map<std::string, uint32_t>* sIndex = nullptr;
Slot* sSlots[kMaxSlots] = {};
std::atomic<uint64_t> sFallback{ 0 };
uint64_t sListHash = 0;

struct PauseScope {
    PauseScope() {
        tPaused++;
    }
    ~PauseScope() {
        tPaused--;
    }
};

uint64_t Fnv(const std::string& s, uint64_t h) {
    for (unsigned char c : s) {
        h ^= c;
        h *= 0x100000001B3ULL;
    }
    return h;
}

#ifdef __EMSCRIPTEN__
// Slot size = k * file size + c, by the resource type in the file header (bytes 4..7, as stored).
struct SizeFactor {
    const char* type;
    float k;
    uint32_t c;
};
const SizeFactor kFactors[] = {
    { "XETO", 1.5f, 2048 },  { "tex ", 1.5f, 2048 },  { "IGBO", 1.5f, 4096 }, { "BLSO", 2.0f, 3072 },
    { "BLBO", 2.0f, 2048 },  { "TLDO", 11.0f, 6144 }, { "RRAO", 7.0f, 6144 }, { "MORO", 8.0f, 6144 },
    { "LKSO", 9.0f, 6144 },  { "MNAO", 2.0f, 8192 },  { "HTPO", 2.0f, 3072 }, { "TVCO", 2.0f, 8192 },
    { "LOCO", 3.0f, 49152 }, { "MAPO", 3.0f, 12288 }, { "TXTO", 7.0f, 8192 }, { "play", 3.0f, 6144 },
};

uint64_t WasmSlotSize(uint32_t fileSize, const char type[4]) {
    float k = 1.0f;
    uint32_t c = 512; // (not a game resource: fonts, images, json... never parsed through a wrapped factory)
    bool printable = true;
    for (int i = 0; i < 4; i++) {
        printable = printable && type[i] >= 32 && type[i] < 127;
    }
    if (printable) {
        k = 10.0f; // (a game resource type not measured yet: generous)
        c = 16384;
        for (const auto& f : kFactors) {
            if (std::memcmp(f.type, type, 4) == 0) {
                k = f.k;
                c = f.c;
                break;
            }
        }
    }
    uint64_t n = (uint64_t)(k * (float)fileSize) + c;
    return (n + 63) & ~(uint64_t)63;
}

// Called from BuildIndex: sizes and types of every file of the loaded archives (later archives override earlier
// ones, as the virtual file system does), then the slot layout in sorted-path order.
void BuildWasmLayout(const std::vector<std::string>& sorted) {
    auto t0 = std::chrono::steady_clock::now();
    struct Meta {
        uint32_t size;
        char type[4];
    };
    std::unordered_map<std::string, Meta> meta;
    meta.reserve(sorted.size());
    auto archives = Ship::Context::GetRawInstance()->GetResourceManager()->GetArchiveManager()->GetArchives();
    for (const auto& archive : *archives) {
        int zerr = 0;
        zip_t* za = zip_open(archive->GetPath().c_str(), ZIP_RDONLY, &zerr);
        if (za == nullptr) {
            Log("zmp: resource slots: cannot open " + archive->GetPath() + " (libzip error " + std::to_string(zerr) +
                ")");
            continue;
        }
        zip_int64_t n = zip_get_num_entries(za, 0);
        for (zip_int64_t i = 0; i < n; i++) {
            zip_stat_t st;
            zip_stat_init(&st);
            if (zip_stat_index(za, i, 0, &st) != 0 || !(st.valid & ZIP_STAT_NAME) || !(st.valid & ZIP_STAT_SIZE)) {
                continue;
            }
            Meta m{ (uint32_t)st.size, { 0, 0, 0, 0 } };
            if (st.size >= 8) {
                if (zip_file_t* zf = zip_fopen_index(za, i, 0)) {
                    unsigned char hdr[8];
                    if (zip_fread(zf, hdr, 8) == 8) {
                        std::memcpy(m.type, hdr + 4, 4);
                    }
                    zip_fclose(zf);
                }
            }
            meta[st.name] = m;
        }
        zip_close(za);
    }
    sWasmOffsets.assign(sorted.size(), 0);
    sWasmSizes.assign(sorted.size(), 0);
    uint64_t offset = 0;
    uint32_t unknown = 0;
    for (size_t i = 0; i < sorted.size(); i++) {
        auto it = meta.find(sorted[i]);
        uint64_t slot;
        if (it == meta.end()) {
            unknown++;
            slot = 16384;
        } else {
            slot = WasmSlotSize(it->second.size, it->second.type);
        }
        sWasmOffsets[i] = offset;
        sWasmSizes[i] = slot;
        offset += slot;
    }
    sActive = offset <= kWasmRegionSize;
    auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(std::chrono::steady_clock::now() - t0).count();
    Log("zmp: resource slots (web): " + std::to_string(sorted.size()) + " files, layout of " +
        std::to_string(offset >> 20) + " MiB in a region of " + std::to_string(kWasmRegionSize >> 20) + " MiB" +
        (unknown ? ", " + std::to_string(unknown) + " files without metadata" : "") + ", built in " +
        std::to_string(ms) + " ms" + (sActive ? "" : " -> DOES NOT FIT, slots off"));
}
#endif

// Called with sMutex held and allocations paused.
void BuildIndex() {
    if (sIndexBuilt) {
        return;
    }
    sIndexBuilt = true;
    auto files = Ship::Context::GetRawInstance()->GetResourceManager()->GetArchiveManager()->ListFiles();
    std::vector<std::string> sorted(files->begin(), files->end());
    std::sort(sorted.begin(), sorted.end());
    sorted.erase(std::unique(sorted.begin(), sorted.end()), sorted.end());
    sIndex = new std::unordered_map<std::string, uint32_t>();
    sIndex->reserve(sorted.size());
    uint64_t h = 0xCBF29CE484222325ULL;
    for (size_t i = 0; i < sorted.size() && i < kMaxSlots; i++) {
        (*sIndex)[sorted[i]] = (uint32_t)i;
        h = Fnv(sorted[i], h);
        h = Fnv("\n", h);
    }
    sListHash = h;
#ifdef _WIN32
    // The whole region is reserved lazily, slot by slot; check the first slot address is usable.
    void* probe = VirtualAlloc((void*)RegionBaseAddr(), kSlotSize, MEM_RESERVE, PAGE_READWRITE);
    sActive = probe == (void*)RegionBaseAddr() && std::getenv("ZMP_NO_RESOURCE_SLOTS") == nullptr;
    if (probe != nullptr) {
        VirtualFree(probe, 0, MEM_RELEASE);
    }
#elif defined(__EMSCRIPTEN__)
    BuildWasmLayout(sorted);
#endif
    Log("zmp: resource slots " + std::string(sActive ? "active" : "NOT available") + ", " +
        std::to_string(sorted.size()) + " archive files" + (sorted.size() > kMaxSlots ? " (TRUNCATED)" : ""));
}

// Returns the slot for a resource path or nullptr. Called with allocations paused.
Slot* AcquireSlot(const std::string& path) {
    std::lock_guard<std::mutex> lock(sMutex);
    BuildIndex();
    if (!sActive) {
        return nullptr;
    }
    auto it = sIndex->find(path);
    if (it == sIndex->end()) {
        return nullptr;
    }
    uint32_t idx = it->second;
    Slot* s = sSlots[idx];
    if (s == nullptr) {
        uint64_t base = RegionBaseAddr() + (uint64_t)idx * kSlotSize;
        uint64_t size = kSlotSize;
#ifdef _WIN32
        void* p = VirtualAlloc((void*)base, kSlotSize, MEM_RESERVE, PAGE_READWRITE);
        if (p != (void*)base) {
            if (p != nullptr) {
                VirtualFree(p, 0, MEM_RELEASE);
            }
            Log("zmp: resource slot " + std::to_string(idx) + " could not be reserved for " + path);
            return nullptr;
        }
#elif defined(__EMSCRIPTEN__)
        if (idx >= sWasmOffsets.size()) {
            return nullptr;
        }
        base = RegionBaseAddr() + sWasmOffsets[idx];
        size = sWasmSizes[idx];
#else
        return nullptr;
#endif
        s = new Slot();
        s->base = base;
        s->size = size;
        s->index = idx;
        s->path = path;
        sSlots[idx] = s;
    }
    bool expected = false;
    if (!s->busy.compare_exchange_strong(expected, true)) {
        Log("zmp: resource slot busy (concurrent load of " + path + "), using the normal allocator");
        return nullptr;
    }
    if (s->loads > 0) {
        Log("zmp: resource " + path + " parsed again (" + std::to_string(s->loads + 1) +
            " times); its new copy continues the slot");
    }
    s->loads++;
    return s;
}

void ReleaseSlot(Slot* s) {
    s->busy.store(false);
}

class SlotFactory : public Ship::ResourceFactory {
  public:
    explicit SlotFactory(std::shared_ptr<Ship::ResourceFactory> inner) : mInner(std::move(inner)) {
    }

    std::shared_ptr<Ship::IResource> ReadResource(std::shared_ptr<Ship::File> file,
                                                  std::shared_ptr<Ship::ResourceInitData> initData) override {
        Slot* slot = nullptr;
        {
            PauseScope pause;
            slot = AcquireSlot(initData->Path);
        }
        if (slot == nullptr) {
            return mInner->ReadResource(file, initData);
        }
        Slot* prevSlot = tSlot;
        int prevPaused = tPaused;
        tSlot = slot;
        tPaused = 0;
        struct Restore {
            Slot* slot;
            Slot* prevSlot;
            int prevPaused;
            ~Restore() {
                tSlot = prevSlot;
                tPaused = prevPaused;
                ReleaseSlot(slot);
            }
        } restore{ slot, prevSlot, prevPaused };
        // The archive read the file into a buffer before this factory runs, outside the slot. Some resources keep
        // pointing into that buffer instead of copying it (textures: their pixels; D-043), so heap data that points
        // at them would hold a different address in every process. The resource gets a copy of the buffer made
        // inside its slot: same bytes, same address everywhere. The original buffer stays alive while the reader,
        // which streams from it, parses.
        std::shared_ptr<std::vector<char>> original = file->Buffer;
        if (original != nullptr) {
            file->Buffer = std::make_shared<std::vector<char>>(original->begin(), original->end());
        }
        return mInner->ReadResource(file, initData);
    }

  protected:
    bool FileHasValidFormatAndReader(std::shared_ptr<Ship::File>, std::shared_ptr<Ship::ResourceInitData>) override {
        return true; // the wrapped factory checks it inside ReadResource
    }

  private:
    std::shared_ptr<Ship::ResourceFactory> mInner;
};

} // namespace

namespace detail {

void* TryAlloc(size_t size, size_t align) {
    Slot* s = tSlot;
    if (s == nullptr || tPaused != 0) {
        return nullptr;
    }
    if (size == 0) {
        size = 1;
    }
    if (align < 16) {
        align = 16;
    }
    uint64_t start = (s->base + s->used + (align - 1)) & ~(uint64_t)(align - 1);
    uint64_t end = start + size;
    if (end > s->base + s->size) {
        sFallback++;
        return nullptr;
    }
    if (end > s->base + s->committed) {
        uint64_t newCommitted = ((end - s->base) + kPage - 1) & ~(kPage - 1);
#ifdef _WIN32
        if (VirtualAlloc((void*)(s->base + s->committed), newCommitted - s->committed, MEM_COMMIT, PAGE_READWRITE) ==
            nullptr) {
            sFallback++;
            return nullptr;
        }
#endif
        s->committed = newCommitted;
    }
    s->used = end - s->base;
    return (void*)start;
}

} // namespace detail

bool Owns(const void* p) {
    uint64_t a = (uint64_t)(uintptr_t)p;
    return a >= RegionBaseAddr() && a < RegionEndAddr();
}

uint64_t RegionBase() {
    return RegionBaseAddr();
}

std::shared_ptr<Ship::ResourceFactory> Wrap(std::shared_ptr<Ship::ResourceFactory> factory) {
    return std::make_shared<SlotFactory>(std::move(factory));
}

std::shared_ptr<Ship::IResource> NestedLoad(const std::string& path) {
    PauseScope pause;
    return Ship::Context::GetRawInstance()->GetResourceManager()->LoadResourceProcess(path);
}

std::vector<SlotInfo> ListSlots() {
    PauseScope pause;
    std::lock_guard<std::mutex> lock(sMutex);
    std::vector<SlotInfo> out;
    for (uint32_t i = 0; i < kMaxSlots; i++) {
        Slot* s = sSlots[i];
        if (s != nullptr && s->loads > 0) {
            out.push_back({ s->path, s->index, s->base, s->used, s->loads });
        }
    }
    return out;
}

uint64_t FileListHash() {
    PauseScope pause;
    std::lock_guard<std::mutex> lock(sMutex);
    BuildIndex();
    return sListHash;
}

bool IsActive() {
    PauseScope pause;
    std::lock_guard<std::mutex> lock(sMutex);
    BuildIndex();
    return sActive;
}

uint64_t FallbackCount() {
    return sFallback.load();
}

} // namespace Zmp::ResSlots

// Global allocation functions of the executable. Identical to the defaults (malloc/free) except
// that allocations made while a resource is being parsed go to that resource's slot, and frees of
// slot memory are ignored (slots are never reused).
namespace {

void* ZmpNew(size_t n) {
    if (void* p = Zmp::ResSlots::detail::TryAlloc(n, 16)) {
        return p;
    }
    for (;;) {
        void* p = std::malloc(n ? n : 1);
        if (p != nullptr) {
            return p;
        }
        std::new_handler h = std::get_new_handler();
        if (h == nullptr) {
            throw std::bad_alloc();
        }
        h();
    }
}

void* ZmpNewAligned(size_t n, std::align_val_t al) {
    size_t align = (size_t)al;
    if (void* p = Zmp::ResSlots::detail::TryAlloc(n, align)) {
        return p;
    }
    for (;;) {
#ifdef _WIN32
        void* p = _aligned_malloc(n ? n : 1, align);
#else
        void* p = std::aligned_alloc(align, ((n ? n : 1) + align - 1) / align * align);
#endif
        if (p != nullptr) {
            return p;
        }
        std::new_handler h = std::get_new_handler();
        if (h == nullptr) {
            throw std::bad_alloc();
        }
        h();
    }
}

void ZmpDelete(void* p) noexcept {
    if (p == nullptr || Zmp::ResSlots::Owns(p)) {
        return;
    }
    std::free(p);
}

void ZmpDeleteAligned(void* p) noexcept {
    if (p == nullptr || Zmp::ResSlots::Owns(p)) {
        return;
    }
#ifdef _WIN32
    _aligned_free(p);
#else
    std::free(p);
#endif
}

} // namespace

void* operator new(size_t n) {
    return ZmpNew(n);
}
void* operator new[](size_t n) {
    return ZmpNew(n);
}
void* operator new(size_t n, const std::nothrow_t&) noexcept {
    try {
        return ZmpNew(n);
    } catch (...) { return nullptr; }
}
void* operator new[](size_t n, const std::nothrow_t&) noexcept {
    try {
        return ZmpNew(n);
    } catch (...) { return nullptr; }
}
void operator delete(void* p) noexcept {
    ZmpDelete(p);
}
void operator delete[](void* p) noexcept {
    ZmpDelete(p);
}
void operator delete(void* p, size_t) noexcept {
    ZmpDelete(p);
}
void operator delete[](void* p, size_t) noexcept {
    ZmpDelete(p);
}
void operator delete(void* p, const std::nothrow_t&) noexcept {
    ZmpDelete(p);
}
void operator delete[](void* p, const std::nothrow_t&) noexcept {
    ZmpDelete(p);
}
void* operator new(size_t n, std::align_val_t al) {
    return ZmpNewAligned(n, al);
}
void* operator new[](size_t n, std::align_val_t al) {
    return ZmpNewAligned(n, al);
}
void* operator new(size_t n, std::align_val_t al, const std::nothrow_t&) noexcept {
    try {
        return ZmpNewAligned(n, al);
    } catch (...) { return nullptr; }
}
void* operator new[](size_t n, std::align_val_t al, const std::nothrow_t&) noexcept {
    try {
        return ZmpNewAligned(n, al);
    } catch (...) { return nullptr; }
}
void operator delete(void* p, std::align_val_t) noexcept {
    ZmpDeleteAligned(p);
}
void operator delete[](void* p, std::align_val_t) noexcept {
    ZmpDeleteAligned(p);
}
void operator delete(void* p, size_t, std::align_val_t) noexcept {
    ZmpDeleteAligned(p);
}
void operator delete[](void* p, size_t, std::align_val_t) noexcept {
    ZmpDeleteAligned(p);
}
void operator delete(void* p, std::align_val_t, const std::nothrow_t&) noexcept {
    ZmpDeleteAligned(p);
}
void operator delete[](void* p, std::align_val_t, const std::nothrow_t&) noexcept {
    ZmpDeleteAligned(p);
}
