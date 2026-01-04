import React from 'react';
import { Button } from '@acme/ui';

export function App() {
  return (
    <div className="app">
      <h1>ACME Dashboard</h1>
      <Button onClick={() => console.log('clicked')}>
        Click me
      </Button>
    </div>
  );
}
