# GopherBin Web UI - Svelte + Tailwind CSS

Modern web interface for GopherBin built with Svelte, SvelteKit, and Tailwind CSS.

## Features

- **Authentication**: Login/logout with JWT tokens, auto-login from localStorage
- **Paste Management**:
  - Create pastes with ACE code editor
  - View pastes with syntax highlighting
  - List pastes with pagination
  - Delete pastes
  - Toggle paste visibility (public/private)
  - Set expiration dates
  - Auto-detect language from filename
- **Theme System**:
  - Light and dark modes
  - Manual theme toggle
  - Automatic system preference detection
  - Persistent theme preference
- **Admin Features**:
  - User management (list, create, delete)
  - Paginated user list
  - Admin-only routes
- **Public Access**: View public pastes without authentication

## Technology Stack

- **Framework**: Svelte 5 + SvelteKit 2
- **Build Tool**: Vite 7
- **Language**: TypeScript
- **Styling**: Tailwind CSS 4
- **Code Editor**: ACE Editor
- **Date Handling**: date-fns

## Project Structure

```
src/
├── lib/
│   ├── components/
│   │   ├── ui/              # Reusable UI components
│   │   ├── editor/          # Code editor components
│   │   ├── paste/           # Paste-specific components (not yet implemented)
│   │   ├── admin/           # Admin components (not yet implemented)
│   │   └── layout/          # Layout components
│   ├── stores/              # Svelte stores (auth, theme)
│   ├── api/                 # API client functions
│   ├── utils/               # Utility functions
│   └── types/               # TypeScript type definitions
├── routes/                  # SvelteKit file-based routes
│   ├── login/              # Login page
│   ├── logout/             # Logout handler
│   ├── p/                  # Paste routes
│   │   ├── [id]/          # View private paste
│   │   └── +page.svelte   # List pastes
│   ├── public/p/[id]/     # View public paste
│   ├── admin/users/        # Admin user management
│   └── +page.svelte        # Home (create paste)
├── app.css                  # Global styles + Tailwind imports
└── app.html                 # HTML template
```

## Development

### Prerequisites

- Node.js 18+ and npm
- GopherBin backend server running on port 9997

### Install Dependencies

```bash
npm install
```

### Development Server

```bash
npm run dev
```

The app will be available at [http://localhost:3000](http://localhost:3000). The dev server proxies API requests to `http://localhost:9997`.

### Build for Production

```bash
npm run build
```

The built files will be in the `build/` directory.

### Preview Production Build

```bash
npm run preview
```

## API Integration

The app communicates with the GopherBin backend API at `/api/v1`. The Vite dev server proxies these requests to the backend server.

### API Endpoints Used

- `POST /api/v1/auth/login` - User login
- `GET /api/v1/logout` - User logout
- `POST /api/v1/paste` - Create paste
- `GET /api/v1/paste` - List pastes
- `GET /api/v1/paste/{id}` - Get paste
- `PUT /api/v1/paste/{id}` - Update paste
- `DELETE /api/v1/paste/{id}` - Delete paste
- `GET /api/v1/public/paste/{id}` - Get public paste
- `GET /api/v1/admin/users` - List users (admin only)
- `POST /api/v1/admin/users` - Create user (admin only)
- `DELETE /api/v1/admin/users/{id}` - Delete user (admin only)

## Configuration

### API Proxy

The Vite dev server proxies API requests. To change the backend URL, edit `vite.config.ts`:

```typescript
server: {
  port: 3000,
  proxy: {
    '/api': {
      target: 'http://localhost:9997', // Change this to your backend URL
      changeOrigin: true
    }
  }
}
```

### Tailwind Configuration

Tailwind CSS is configured in `tailwind.config.js`. Custom colors and theme settings can be modified there.

## State Management

### Auth Store

Manages authentication state:
- Token storage in localStorage
- Auto-login on page load
- Admin status tracking
- User information

### Theme Store

Manages theme preference:
- Light/dark mode
- System preference detection
- localStorage persistence
- Automatic theme application

## Features Not Yet Implemented

While the core functionality is complete, the following features from the original React app could be added:

1. **Paste Sharing**: Share pastes with specific users
2. **Teams**: Team management and team-based paste sharing
3. **Paste Download**: Download paste content as file
4. **User Profile Editing**: Edit user details
5. **Advanced Filters**: Filter pastes by language, date, etc.
6. **Search**: Search through pastes

## Deployment

### With GopherBin Backend

1. Build the app: `npm run build`
2. Copy the `build/` directory contents to the GopherBin static assets location
3. Update the GopherBin backend to serve the new UI

### Standalone

The app can also be deployed to any static hosting service (Vercel, Netlify, etc.) by pointing the API proxy to your backend server.

## Development Notes

- The app uses SvelteKit's static adapter for deployment as a SPA
- All routes are client-side rendered (CSR)
- The ACE editor includes only the most common language modes to reduce bundle size
- Authentication is handled entirely client-side via JWT tokens
- No server-side rendering (SSR) is used

## Browser Support

- Modern evergreen browsers (Chrome, Firefox, Safari, Edge)
- Requires JavaScript enabled
- Requires localStorage support

## License

Apache License 2.0 (same as GopherBin)
